package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func organizeMetadataMatchTrusted(query string, sourceYear int, match *Match) bool {
	if match == nil || strings.TrimSpace(match.Title) == "" {
		return false
	}
	if unsafeAutomaticEpisodeQuery(query) {
		return false
	}
	if sourceYear > 0 && match.Year > 0 {
		diff := sourceYear - match.Year
		if diff < 0 {
			diff = -diff
		}
		if diff > 1 {
			return false
		}
	}
	return automaticMetadataTitleTrusted(query, match)
}

func organizeMatchFromLocalMetadata(local *LocalMetadata) *Match {
	if local == nil || strings.TrimSpace(local.Title) == "" {
		return nil
	}
	match := &Match{
		Title:        strings.TrimSpace(local.Title),
		OriginalName: strings.TrimSpace(local.OriginalName),
		Overview:     local.Overview,
		PosterURL:    local.PosterURL,
		BackdropURL:  local.BackdropURL,
		Year:         local.Year,
		ReleaseDate:  local.ReleaseDate,
		Rating:       local.Rating,
		TMDbID:       local.TMDbID,
		DoubanID:     local.DoubanID,
		TheTVDBID:    local.TheTVDBID,
		NSFW:         local.NSFW,
	}
	if local.Genres != "" {
		match.Genres = splitNFOList(local.Genres)
	}
	if local.Countries != "" {
		match.Countries = splitNFOList(local.Countries)
	}
	if local.Languages != "" {
		match.Languages = splitNFOList(local.Languages)
	}
	return match
}

func (o *OrganizerService) lookupOrganizeSourceMedia(ctx context.Context, path string) *model.Media {
	if o == nil || o.repo == nil || o.repo.DB == nil {
		return nil
	}
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return nil
	}
	var media model.Media
	if err := o.repo.DB.WithContext(ctx).
		Where("path = ? AND deleted_at IS NULL", path).
		Limit(1).
		Take(&media).Error; err != nil {
		return nil
	}
	return &media
}

func organizeMatchFromMedia(media *model.Media) *Match {
	if media == nil || strings.TrimSpace(media.Title) == "" {
		return nil
	}
	return &Match{
		TMDbID:       media.TMDbID,
		BangumiID:    media.BangumiID,
		DoubanID:     strings.TrimSpace(media.DoubanID),
		TheTVDBID:    strings.TrimSpace(media.TheTVDBID),
		Title:        strings.TrimSpace(media.Title),
		OriginalName: strings.TrimSpace(media.OriginalName),
		Overview:     media.Overview,
		PosterURL:    media.PosterURL,
		BackdropURL:  media.BackdropURL,
		Year:         media.Year,
		ReleaseDate:  media.ReleaseDate,
		Rating:       media.Rating,
		Languages:    parseCommaList(media.Languages),
		Countries:    parseCommaList(media.Countries),
		Genres:       parseCommaList(media.Genres),
		NSFW:         media.NSFW,
	}
}

func applyOrganizeMetadataMatch(match *Match, title, parsedTitle *string, year *int) {
	if match == nil {
		return
	}
	if matchedTitle := sanitizeFilename(strings.TrimSpace(match.Title)); matchedTitle != "" {
		*title = matchedTitle
		*parsedTitle = strings.TrimSpace(match.Title)
	}
	if match.Year > 0 {
		*year = match.Year
	}
}

func organizeMetadataCacheKey(mediaType, query string, year int) string {
	return strings.ToLower(strings.TrimSpace(mediaType)) + "|" + fmt.Sprint(year) + "|" + strings.ToLower(strings.TrimSpace(query))
}
