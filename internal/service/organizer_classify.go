package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// isSmartClassifyEnabled checks database settings first, then config.yaml.
func (o *OrganizerService) isSmartClassifyEnabled(ctx context.Context) bool {
	if o.repo != nil && o.repo.Setting != nil {
		val, err := o.repo.Setting.Get(ctx, "organizer.smart_classify")
		if err == nil && val != "" {
			return val == "true" || val == "1" || val == "on"
		}
	}
	if o == nil || o.cfg == nil {
		return false
	}
	return o.cfg.Organizer.SmartClassify
}

// SmartClassify determines the subcategory folder based on media metadata.
// It returns values such as "华语电影", "欧美剧", or "日番".
func (o *OrganizerService) SmartClassify(ctx context.Context, m *model.Media) string {
	if !o.isSmartClassifyEnabled(ctx) {
		return ""
	}
	lib, err := o.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil || lib == nil {
		return ""
	}
	return o.classifyMedia(ctx, m, lib.Type)
}

func (o *OrganizerService) smartClassifySourceFile(ctx context.Context, src, sourceRoot, mediaType, title, parsedTitle string, metadataMatch *Match) string {
	if o == nil || !o.isSmartClassifyEnabled(ctx) {
		return ""
	}
	seriesLike := isSeriesLibraryType(mediaType)
	input := mediaClassifyInput{
		MediaType: mediaType,
		Title:     strings.Join([]string{title, parsedTitle, filepath.Base(src)}, " "),
		Category:  strings.Join(organizeDirectoryCategoryCandidates(src, sourceRoot), " "),
	}
	if metadataMatch != nil {
		input.Title = strings.Join([]string{
			metadataMatch.OriginalName,
			title,
			parsedTitle,
			filepath.Base(src),
		}, " ")
		input.Languages = metadataMatch.Languages
		input.Countries = metadataMatch.Countries
		input.Genres = metadataMatch.Genres
		if metadataMatch.NSFW {
			input.MediaType = "adult"
		}
	}
	if meta, err := ReadLocalMetadata(src, sourceRoot, seriesLike); err == nil && meta != nil && meta.HasNFO {
		input.Title = strings.Join([]string{meta.Title, meta.OriginalName, title, parsedTitle, filepath.Base(src)}, " ")
		input.Languages = parseCommaList(meta.Languages)
		input.Countries = parseCommaList(meta.Countries)
		input.Genres = parseCommaList(meta.Genres)
		if meta.NSFW {
			input.MediaType = "adult"
		}
	}
	return sanitizeFilename(classifyMediaCategory(input, o.categoryMap()))
}

// parseCommaList splits a comma-separated string into trimmed non-empty values.
func parseCommaList(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
