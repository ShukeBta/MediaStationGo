package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (o *OrganizerService) lookupReclassifyMetadata(ctx context.Context, media model.Media, lib model.Library, mediaType string) *Match {
	if o == nil || o.scraper == nil || !o.scraper.AnyEnabled() {
		return nil
	}
	title := strings.TrimSpace(media.Title)
	if title == "" {
		title, _ = CleanQuery(media.Path)
	}
	for _, typ := range reclassifyMetadataLookupTypes(mediaType, media) {
		if match := o.lookupOrganizeMetadata(ctx, media.Path, lib.Path, typ, title, media.Year, media.SeasonNum, media.EpisodeNum, nil); match != nil {
			if o.log != nil {
				o.log.Info("metadata category reclassify filled missing metadata",
					zap.String("media", media.ID),
					zap.String("path", media.Path),
					zap.String("title", match.Title),
					zap.String("media_type", typ),
					zap.Int("tmdb_id", match.TMDbID),
					zap.Int("bangumi_id", match.BangumiID),
					zap.String("douban_id", match.DoubanID),
					zap.String("thetvdb_id", match.TheTVDBID))
			}
			return match
		}
	}
	return nil
}

func reclassifyMetadataLookupTypes(mediaType string, media model.Media) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	add := func(value string) {
		value = normalizeOrganizeMediaType(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	add(mediaType)
	if media.SeasonNum > 0 || media.EpisodeNum > 0 {
		add("tv")
		add("anime")
	}
	switch normalizeOrganizeMediaType(mediaType) {
	case "tv":
		add("anime")
		add("movie")
	case "anime":
		add("tv")
		add("movie")
	case "movie", "":
		add("tv")
		add("anime")
		add("movie")
	default:
		add("tv")
		add("movie")
	}
	return out
}

func mediaWithReclassifyMatch(media model.Media, match *Match) model.Media {
	if match == nil {
		return media
	}
	if value := strings.TrimSpace(match.Title); value != "" {
		media.Title = value
	}
	if value := strings.TrimSpace(match.OriginalName); value != "" {
		media.OriginalName = value
	}
	if match.Year > 0 {
		media.Year = match.Year
	}
	if value := strings.TrimSpace(match.ReleaseDate); value != "" {
		media.ReleaseDate = value
	}
	if match.TMDbID > 0 {
		media.TMDbID = match.TMDbID
	}
	if match.BangumiID > 0 {
		media.BangumiID = match.BangumiID
	}
	if value := strings.TrimSpace(match.DoubanID); value != "" {
		media.DoubanID = value
	}
	if value := strings.TrimSpace(match.TheTVDBID); value != "" {
		media.TheTVDBID = value
	}
	if len(match.Languages) > 0 {
		media.Languages = strings.Join(match.Languages, ",")
	}
	if len(match.Countries) > 0 {
		media.Countries = strings.Join(match.Countries, ",")
	}
	if len(match.Genres) > 0 {
		media.Genres = strings.Join(match.Genres, ",")
	}
	if match.NSFW {
		media.NSFW = true
	}
	media.ScrapeStatus = "matched"
	return media
}

func metadataMatchMediaType(match *Match) string {
	if match == nil {
		return ""
	}
	return match.MediaType
}

func mediaHasReliableCategoryMetadata(media model.Media) bool {
	return media.NSFW ||
		strings.TrimSpace(media.Languages) != "" ||
		strings.TrimSpace(media.Countries) != "" ||
		strings.TrimSpace(media.Genres) != ""
}
