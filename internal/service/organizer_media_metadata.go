package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (o *OrganizerService) refreshOrganizeMediaMetadata(ctx context.Context, media *model.Media, lib *model.Library, requestedType string) {
	if o == nil || media == nil || lib == nil || !organizeMediaNeedsMetadataRefresh(*media) {
		return
	}
	mediaType := normalizeOrganizeMediaType(requestedType)
	if mediaType == "" {
		mediaType = normalizeOrganizeMediaType(lib.Type)
	}
	match := o.lookupReclassifyMetadata(ctx, *media, *lib, mediaType)
	if match == nil {
		return
	}
	refreshed := mediaWithReclassifyMatch(*media, match)
	*media = refreshed
	if o.log != nil {
		o.log.Info("organize media metadata refreshed before rename",
			zap.String("media", media.ID),
			zap.String("path", media.Path),
			zap.String("title", media.Title),
			zap.Int("tmdb_id", media.TMDbID),
			zap.Int("bangumi_id", media.BangumiID),
			zap.String("douban_id", media.DoubanID),
			zap.String("thetvdb_id", media.TheTVDBID))
	}
}

func organizeMediaNeedsMetadataRefresh(media model.Media) bool {
	if strings.TrimSpace(media.ScrapeStatus) != "matched" {
		return true
	}
	if organizeMediaTitleLooksLikeRelease(media.Title) {
		return true
	}
	return media.TMDbID <= 0 &&
		media.BangumiID <= 0 &&
		strings.TrimSpace(media.DoubanID) == "" &&
		strings.TrimSpace(media.TheTVDBID) == ""
}

func organizeMediaTitleLooksLikeRelease(title string) bool {
	title = strings.TrimSpace(title)
	if title == "" || organizeWeakFileTitle(title) {
		return true
	}
	if season, episode := ParseEpisode(title); season > 0 || episode > 0 {
		return true
	}
	normalized := strings.ToLower(strings.NewReplacer(".", " ", "_", " ", "-", " ").Replace(title))
	for _, field := range strings.Fields(normalized) {
		if _, ok := releaseBoundaryTokenSet[field]; ok {
			return true
		}
	}
	return false
}

func addOrganizedMediaMetadataUpdates(updates map[string]any, media model.Media) {
	if updates == nil || strings.TrimSpace(media.ScrapeStatus) != "matched" {
		return
	}
	setNonEmptyUpdate(updates, "title", media.Title)
	setNonEmptyUpdate(updates, "original_name", media.OriginalName)
	setNonEmptyUpdate(updates, "episode_title", media.EpisodeTitle)
	setNonEmptyUpdate(updates, "poster_url", media.PosterURL)
	setNonEmptyUpdate(updates, "backdrop_url", media.BackdropURL)
	setNonEmptyUpdate(updates, "overview", media.Overview)
	setNonEmptyUpdate(updates, "languages", media.Languages)
	setNonEmptyUpdate(updates, "countries", media.Countries)
	setNonEmptyUpdate(updates, "genres", media.Genres)
	if media.Year > 0 {
		updates["year"] = media.Year
	}
	setNonEmptyUpdate(updates, "release_date", media.ReleaseDate)
	if media.Rating > 0 {
		updates["rating"] = media.Rating
	}
	if media.TMDbID > 0 {
		updates["tm_db_id"] = media.TMDbID
	}
	if media.BangumiID > 0 {
		updates["bangumi_id"] = media.BangumiID
	}
	setNonEmptyUpdate(updates, "douban_id", media.DoubanID)
	setNonEmptyUpdate(updates, "thetvdb_id", media.TheTVDBID)
	if media.NSFW {
		updates["nsfw"] = true
	}
	updates["scrape_status"] = "matched"
}

func (o *OrganizerService) persistOrganizedMediaMetadata(ctx context.Context, media *model.Media) error {
	if o == nil || o.repo == nil || o.repo.DB == nil || media == nil {
		return nil
	}
	updates := map[string]any{}
	addOrganizedMediaMetadataUpdates(updates, *media)
	if len(updates) == 0 {
		return nil
	}
	return o.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", media.ID).Updates(updates).Error
}

func setNonEmptyUpdate(updates map[string]any, key, value string) {
	if strings.TrimSpace(value) != "" {
		updates[key] = value
	}
}
