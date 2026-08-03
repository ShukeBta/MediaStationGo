package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// MediaCategoryReclassifyOptions controls the metadata-based category audit.
// Empty LibraryIDs means all enabled local libraries.
type MediaCategoryReclassifyOptions struct {
	LibraryIDs     []string
	MediaIDs       []string
	MediaTypeHints map[string]string
	DryRun         bool
}

// ReclassifyMisclassifiedMedia corrects already-scanned local media whose
// stored metadata clearly disagrees with the current library/category.
func (o *OrganizerService) ReclassifyMisclassifiedMedia(ctx context.Context, opts MediaCategoryReclassifyOptions) (*OrganizeResult, error) {
	res := &OrganizeResult{DryRun: opts.DryRun}
	if o == nil || o.repo == nil || o.repo.DB == nil || o.repo.Library == nil {
		return res, nil
	}
	libraries, err := o.repo.Library.List(ctx)
	if err != nil {
		return res, err
	}
	filterIDs := compactLibraryIDs(opts.LibraryIDs...)
	mediaIDs := compactLibraryIDs(opts.MediaIDs...)
	filter := map[string]struct{}{}
	for _, id := range filterIDs {
		filter[id] = struct{}{}
	}
	typeHints := normalizeReclassifyMediaTypeHints(opts.MediaTypeHints)
	libByID := make(map[string]model.Library, len(libraries))
	for _, lib := range libraries {
		if !lib.Enabled || strings.TrimSpace(lib.ID) == "" {
			continue
		}
		if len(filter) > 0 {
			if _, ok := filter[lib.ID]; !ok {
				continue
			}
		}
		libByID[lib.ID] = lib
	}
	if len(libByID) == 0 {
		return res, nil
	}

	query := o.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("deleted_at IS NULL")
	if len(filter) > 0 {
		query = query.Where("library_id IN ?", filterIDs)
	}
	if len(mediaIDs) > 0 {
		query = query.Where("id IN ?", mediaIDs)
	}
	var rows []model.Media
	err = query.FindInBatches(&rows, 500, func(_ *gorm.DB, _ int) error {
		for i := range rows {
			lib, ok := libByID[rows[i].LibraryID]
			if !ok {
				continue
			}
			changed, err := o.reclassifyScannedMedia(ctx, rows[i], lib, typeHints[rows[i].ID], OrganizeOptions{}, opts.DryRun, res)
			if err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", rows[i].Title, err.Error()))
				if o.log != nil {
					o.log.Warn("metadata category reclassify failed",
						zap.String("media", rows[i].ID),
						zap.String("path", rows[i].Path),
						zap.Error(err))
				}
				continue
			}
			if changed && o.log != nil {
				o.log.Debug("metadata category reclassify applied",
					zap.String("media", rows[i].ID),
					zap.String("title", rows[i].Title))
			}
		}
		return nil
	}).Error
	return res, err
}

func normalizeReclassifyMediaTypeHints(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for id, value := range values {
		id = strings.TrimSpace(id)
		mediaType := normalizeOrganizeMediaType(value)
		if id != "" && mediaType != "" {
			out[id] = mediaType
		}
	}
	return out
}

func (o *OrganizerService) reclassifyScannedMedia(ctx context.Context, media model.Media, lib model.Library, mediaTypeHint string, opts OrganizeOptions, dryRun bool, res *OrganizeResult) (bool, error) {
	if res == nil || !lib.Enabled || strings.TrimSpace(media.Path) == "" {
		return false, nil
	}
	if mount, ok := ParseCloudLibraryMount(lib.Path); ok {
		return o.reclassifyCloudScannedMedia(ctx, media, lib, mount, mediaTypeHint, dryRun, res)
	}
	if !organizeFileExists(media.Path) {
		return false, nil
	}

	requestedBaseRoot := o.resolveBaseRoot(ctx, &lib, opts.DestPath)
	overrideType, overrideCategory := o.effectiveOrganizeOverrides(opts, requestedBaseRoot)
	explicitType := overrideType != ""
	explicitCategory := overrideCategory != ""

	mediaType := normalizeOrganizeMediaType(lib.Type)
	if mediaTypeHint != "" {
		mediaType = mediaTypeHint
	}
	if explicitType {
		mediaType = overrideType
	}
	metadataMatch := organizeMatchFromMedia(&media)
	metadataRefreshed := false
	if metadataMatch != nil && mediaTypeHint != "" {
		metadataMatch.MediaType = mediaTypeHint
	}
	if !mediaHasReliableCategoryMetadata(media) {
		metadataMatch = o.lookupReclassifyMetadata(ctx, media, lib, mediaType)
		if metadataMatch == nil {
			return false, nil
		}
		media = mediaWithReclassifyMatch(media, metadataMatch)
		metadataRefreshed = true
	}
	if !explicitType {
		if matchType := normalizeOrganizeMediaType(metadataMatchMediaType(metadataMatch)); matchType != "" {
			mediaType = matchType
		}
	}
	category := overrideCategory
	if category == "" {
		category = o.classifyMedia(ctx, &media, mediaType)
		if isUnclassifiedTVCategory(category, o.categoryMap()) && !mediaHasReliableCategoryMetadata(media) {
			if existingType, existingCategory := o.mediaTypeForDirectoryCategory(firstNonEmpty(lib.Name, filepath.Base(lib.Path))); existingType != "" && sourceCategoryCompatible(mediaType, existingType) {
				category = existingCategory
			}
		}
	}
	if category == "" {
		return false, nil
	}
	if impliedType, normalizedCategory := o.mediaTypeForDirectoryCategory(category); impliedType != "" {
		category = normalizedCategory
		if !explicitType {
			mediaType = impliedType
		}
	}
	if explicitCategory && overrideCategory != "" {
		category = overrideCategory
	}
	if mediaType == "" {
		mediaType = normalizeOrganizeMediaType(lib.Type)
	}
	if mediaType == "" {
		return false, nil
	}

	baseRoot := normalizeMappedOrganizeDestinationRoot(requestedBaseRoot)
	targetLibrary, matched := o.organizeLibraryForLayout(ctx, baseRoot, mediaType, category)
	if !matched || strings.TrimSpace(targetLibrary.ID) == "" || strings.TrimSpace(targetLibrary.Path) == "" {
		targetRoot := categoryRoot(o.organizeRoot(baseRoot, mediaType, category), category)
		if dryRun {
			targetLibrary = model.Library{Path: targetRoot}
		} else if created, ok := o.ensureOrganizeLibraryForRoot(ctx, targetRoot, mediaType, category); ok {
			targetLibrary = created
		} else {
			return false, nil
		}
	}
	if strings.EqualFold(targetLibrary.ID, lib.ID) && pathWithin(media.Path, targetLibrary.Path) {
		if metadataRefreshed && !dryRun {
			if err := o.persistOrganizedMediaMetadata(ctx, &media); err != nil {
				return false, err
			}
			if o.log != nil {
				o.log.Info("media metadata refreshed without reclassify",
					zap.String("media", media.ID),
					zap.String("path", media.Path),
					zap.String("title", media.Title),
					zap.Int("tmdb_id", media.TMDbID),
					zap.Int("bangumi_id", media.BangumiID),
					zap.String("douban_id", media.DoubanID),
					zap.String("thetvdb_id", media.TheTVDBID))
			}
			return true, nil
		}
		return false, nil
	}
	if pathWithin(media.Path, targetLibrary.Path) {
		return o.reclassifyScannedMediaLibraryOnly(ctx, media, lib, targetLibrary, category, mediaType, dryRun, res, metadataMatch)
	}

	title := sanitizeFilename(strings.TrimSpace(media.Title))
	if title == "" {
		title = "Unknown"
	}
	target, err := o.buildOrganizeTargetPath(ctx, organizeTargetInput{
		Root:      targetLibrary.Path,
		MediaType: mediaType,
		Category:  category,
		Title:     title,
		Source:    media.Path,
		Ext:       filepath.Ext(media.Path),
		Year:      media.Year,
		Season:    media.SeasonNum,
		Episode:   media.EpisodeNum,
		Series:    isSeriesLibraryType(mediaType),
	})
	if err != nil {
		return false, err
	}
	return o.reclassifyExistingMedia(ctx, organizeExistingReclassifyRequest{
		Source:          media.Path,
		Target:          target.Path,
		DestRoot:        firstNonEmpty(baseRoot, lib.Path),
		TargetLibraryID: targetLibrary.ID,
		Existing:        []string{media.Path},
		DryRun:          dryRun,
		MediaType:       mediaType,
		Category:        category,
		Title:           title,
		Year:            media.Year,
		Season:          media.SeasonNum,
		Episode:         media.EpisodeNum,
		MetadataMatch:   metadataMatch,
		Result:          res,
	})
}

func (o *OrganizerService) reclassifyScannedMediaLibraryOnly(ctx context.Context, media model.Media, oldLib, targetLib model.Library, category, mediaType string, dryRun bool, res *OrganizeResult, metadataMatch *Match) (bool, error) {
	res.Items = append(res.Items, OrganizePreviewItem{
		Source:    media.Path,
		Target:    media.Path,
		Action:    "reclassify",
		Reason:    "metadata category library changed",
		MediaType: mediaType,
		Category:  category,
		Title:     media.Title,
	})
	if dryRun {
		res.Reclassified++
		return true, nil
	}
	updates := map[string]any{"library_id": targetLib.ID, "series_id": ""}
	if normalizeOrganizeMediaType(mediaType) == "movie" {
		updates["season_num"] = 0
		updates["episode_num"] = 0
		updates["episode_title"] = ""
	}
	applyReclassifyMatchUpdates(updates, metadataMatch)
	if err := o.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Where("id = ?", media.ID).
		Updates(updates).Error; err != nil {
		return false, err
	}
	if o.log != nil {
		o.log.Info("media library reclassified by metadata",
			zap.String("media", media.ID),
			zap.String("path", media.Path),
			zap.String("from_library", oldLib.ID),
			zap.String("to_library", targetLib.ID),
			zap.String("category", category),
			zap.String("media_type", mediaType))
	}
	res.Reclassified++
	return true, nil
}
