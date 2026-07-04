package service

import (
	"context"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

func (o *OrganizerService) buildOrganizeSourceFilePlan(ctx context.Context, req organizeSourceFileRequest) (organizeSourceFilePlan, error) {
	identity := o.resolveOrganizeSourceIdentity(ctx, req)
	pathLayout := o.inferOrganizeDirectoryLayout(req.Source, req.SourceRoot)
	layout := pathLayout
	forcedType := normalizeOrganizeMediaType(req.MediaTypeOverride)
	inferredType := o.inferMediaTypeForSourceFile(req.Source, identity.Title, identity.Season, identity.Episode)
	layout = applyOrganizeSourceMediaType(layout, forcedType, inferredType)
	lookupType := organizeSourceMetadataLookupType(req.Source, pathLayout, layout, forcedType, inferredType, identity)
	metadataMatch := o.lookupOrganizeSourceMetadata(ctx, req, lookupType, &identity)
	if metadataMatch == nil && lookupType != layout.MediaType {
		metadataMatch = o.lookupOrganizeSourceMetadata(ctx, req, layout.MediaType, &identity)
	}
	if forcedType == "" && metadataMatch != nil {
		if matchType := normalizeOrganizeMediaType(metadataMatch.MediaType); matchType != "" && matchType != layout.MediaType {
			if categoryType, _ := o.mediaTypeForDirectoryCategory(layout.Category); categoryType != "" && !sourceCategoryCompatible(matchType, categoryType) {
				layout.Category = ""
			}
			if o.log != nil {
				o.log.Info("organize media type corrected by metadata",
					zap.String("source", req.Source),
					zap.String("from", layout.MediaType),
					zap.String("to", matchType),
					zap.String("title", metadataMatch.Title),
					zap.Int("tmdb_id", metadataMatch.TMDbID))
			}
			layout.MediaType = matchType
		}
	}
	layout = o.applyOrganizeSourceCategory(ctx, req, pathLayout, layout, forcedType, identity, metadataMatch)
	layoutRoot, targetLibraryID := o.resolveOrganizeSourceLayoutRoot(ctx, req, layout)
	target, err := o.buildOrganizeSourceTarget(ctx, req, layout, layoutRoot, identity)
	if err != nil {
		return organizeSourceFilePlan{}, err
	}
	return organizeSourceFilePlan{
		Identity:        identity,
		Layout:          layout,
		LayoutRoot:      layoutRoot,
		TargetLibraryID: targetLibraryID,
		MetadataMatch:   metadataMatch,
		Target:          target,
	}, nil
}

func (o *OrganizerService) resolveOrganizeSourceIdentity(ctx context.Context, req organizeSourceFileRequest) organizeSourceIdentity {
	src := req.Source
	ext := filepath.Ext(src)
	season, episode := ParseEpisode(src)
	title, year := CleanQueryWithRecognition(ctx, o.repo, src)
	if organizeWeakFileTitle(title) {
		if folderTitle, folderYear := organizeTitleFromParentFolder(src, req.SourceRoot, season > 0 || episode > 0); folderTitle != "" {
			title = folderTitle
			if year <= 0 {
				year = folderYear
			}
		} else {
			title = strings.TrimSuffix(filepath.Base(src), ext)
		}
	}
	parsedTitle := title
	sourceMedia := o.lookupOrganizeSourceMedia(ctx, src)
	title = sanitizeFilename(titleCaseWords(title))
	if sourceMedia != nil {
		if mediaTitle := sanitizeFilename(strings.TrimSpace(sourceMedia.Title)); mediaTitle != "" {
			title = mediaTitle
			parsedTitle = strings.TrimSpace(sourceMedia.Title)
		}
		if sourceMedia.Year > 0 {
			year = sourceMedia.Year
		}
		if sourceMedia.SeasonNum > 0 {
			season = sourceMedia.SeasonNum
		}
		if sourceMedia.EpisodeNum > 0 {
			episode = sourceMedia.EpisodeNum
		}
	}
	if title == "" {
		title = "Unknown"
	}
	return organizeSourceIdentity{
		Ext:         ext,
		Title:       title,
		ParsedTitle: parsedTitle,
		Year:        year,
		Season:      season,
		Episode:     episode,
		SourceMedia: sourceMedia,
	}
}

func applyOrganizeSourceMediaType(layout organizeDirectoryLayout, forcedType, inferredType string) organizeDirectoryLayout {
	if forcedType != "" {
		if layout.Category != "" && layout.MediaType != "" && layout.MediaType != forcedType {
			layout.Category = ""
		}
		layout.MediaType = forcedType
		return layout
	}
	if inferredType == "" {
		return layout
	}
	if inferredType == "tv" && layout.MediaType == "movie" {
		return organizeDirectoryLayout{MediaType: inferredType}
	}
	if layout.MediaType == "" {
		layout.MediaType = inferredType
	}
	return layout
}

func organizeSourceMetadataLookupType(src string, pathLayout, layout organizeDirectoryLayout, forcedType, inferredType string, identity organizeSourceIdentity) string {
	if forcedType != "" {
		return layout.MediaType
	}
	if pathLayout.MediaType != "" && pathLayout.MediaType != "movie" && organizeStandaloneMovieSourceHint(src, identity) {
		return "movie"
	}
	return layout.MediaType
}

func organizeStandaloneMovieSourceHint(src string, identity organizeSourceIdentity) bool {
	if identity.Season > 0 || identity.Episode > 0 {
		return organizeEpisodeLooksSourcedFromMovieYear(src, identity)
	}
	_, year := CleanQueryWithRecognition(context.Background(), nil, filepath.Base(src))
	if year <= 0 {
		year = identity.Year
	}
	if year <= 0 {
		return false
	}
	text := strings.ToLower(strings.Join([]string{filepath.Base(src), identity.Title, identity.ParsedTitle}, " "))
	return !classifierEpisodeRE.MatchString(text) && !classifierSeasonRE.MatchString(text)
}

func organizeEpisodeLooksSourcedFromMovieYear(src string, identity organizeSourceIdentity) bool {
	year := identity.Year
	if identity.SourceMedia != nil && identity.SourceMedia.Year > 0 {
		year = identity.SourceMedia.Year
	}
	if year < 1900 || year > 2099 || identity.Season != 1 || identity.Episode != year/10 {
		return false
	}
	showDir := showDirFromEpisodePath(src)
	if showDir == "" {
		return false
	}
	_, folderYear := CleanQuery(filepath.Base(showDir))
	return folderYear == year
}

func (o *OrganizerService) lookupOrganizeSourceMetadata(ctx context.Context, req organizeSourceFileRequest, mediaType string, identity *organizeSourceIdentity) *Match {
	match := o.lookupOrganizeMetadata(ctx, req.Source, req.SourceRoot, mediaType, identity.Title, identity.Year, identity.Season, identity.Episode, req.MetadataCache)
	if match == nil && identity.SourceMedia != nil {
		match = organizeMatchFromMedia(identity.SourceMedia)
	}
	if match != nil {
		applyOrganizeMetadataMatch(match, &identity.Title, &identity.ParsedTitle, &identity.Year)
	}
	return match
}

func (o *OrganizerService) applyOrganizeSourceCategory(
	ctx context.Context,
	req organizeSourceFileRequest,
	pathLayout, layout organizeDirectoryLayout,
	forcedType string,
	identity organizeSourceIdentity,
	metadataMatch *Match,
) organizeDirectoryLayout {
	if category := strings.TrimSpace(req.MediaCategoryOverride); category != "" {
		layout.Category = sanitizeFilename(category)
	} else if category := o.smartClassifySourceFile(ctx, req.Source, req.SourceRoot, layout.MediaType, identity.Title, identity.ParsedTitle, metadataMatch); category != "" {
		layout.Category = category
	}
	if impliedType, normalizedCategory := o.mediaTypeForDirectoryCategory(layout.Category); impliedType != "" {
		layout.Category = normalizedCategory
		if forcedType == "" {
			if layout.MediaType == "" || layout.MediaType == "tv" || layout.MediaType == "anime" || pathLayout.Category != layout.Category {
				layout.MediaType = impliedType
			}
		}
	}
	return layout
}

func (o *OrganizerService) resolveOrganizeSourceLayoutRoot(ctx context.Context, req organizeSourceFileRequest, layout organizeDirectoryLayout) (string, string) {
	targetLibrary, matchedLibrary := o.organizeLibraryForLayout(ctx, req.DestRoot, layout.MediaType, layout.Category)
	layoutRoot := targetLibrary.Path
	targetLibraryID := targetLibrary.ID
	if !matchedLibrary && layout.MediaType != "" {
		layoutRoot = o.organizeRoot(req.DestRoot, layout.MediaType, layout.Category)
	}
	if !matchedLibrary && layout.Category != "" {
		layoutRoot = categoryRoot(layoutRoot, sanitizeFilename(layout.Category))
	}
	if !matchedLibrary && !req.DryRun {
		if targetLibrary, ok := o.ensureOrganizeLibraryForRoot(ctx, layoutRoot, layout.MediaType, layout.Category); ok {
			targetLibraryID = targetLibrary.ID
		}
	}
	return layoutRoot, targetLibraryID
}

func (o *OrganizerService) buildOrganizeSourceTarget(
	ctx context.Context,
	req organizeSourceFileRequest,
	layout organizeDirectoryLayout,
	layoutRoot string,
	identity organizeSourceIdentity,
) (organizeTargetPath, error) {
	isSeries := identity.Season > 0 || identity.Episode > 0
	if layout.MediaType != "" {
		isSeries = isSeriesLibraryType(layout.MediaType) && isSeries
	}
	return o.buildOrganizeTargetPath(ctx, organizeTargetInput{
		Root:      layoutRoot,
		MediaType: layout.MediaType,
		Category:  layout.Category,
		Title:     identity.Title,
		Source:    req.Source,
		Ext:       identity.Ext,
		Year:      identity.Year,
		Season:    identity.Season,
		Episode:   identity.Episode,
		Series:    isSeries,
	})
}
