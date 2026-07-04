package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (o *OrganizerService) updateReclassifiedMediaRow(ctx context.Context, oldPath, newPath string, req organizeExistingReclassifyRequest) error {
	if o == nil || o.repo == nil || o.repo.DB == nil {
		return nil
	}
	updates := map[string]any{
		"path": newPath,
	}
	if strings.TrimSpace(req.TargetLibraryID) != "" {
		updates["library_id"] = strings.TrimSpace(req.TargetLibraryID)
	}
	if strings.TrimSpace(req.Title) != "" {
		updates["title"] = strings.TrimSpace(req.Title)
	}
	if req.Year > 0 {
		updates["year"] = req.Year
	}
	if normalizeOrganizeMediaType(req.MediaType) == "movie" {
		updates["season_num"] = 0
		updates["episode_num"] = 0
	} else if req.Season > 0 {
		updates["season_num"] = req.Season
		if req.Episode > 0 {
			updates["episode_num"] = req.Episode
		}
	} else if req.Episode > 0 {
		updates["episode_num"] = req.Episode
	}
	applyReclassifyMatchUpdates(updates, req.MetadataMatch)
	return o.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("path = ?", oldPath).Updates(updates).Error
}

func applyReclassifyMatchUpdates(updates map[string]any, match *Match) {
	if updates == nil || match == nil {
		return
	}
	if value := strings.TrimSpace(match.Title); value != "" {
		updates["title"] = value
	}
	if value := strings.TrimSpace(match.OriginalName); value != "" {
		updates["original_name"] = value
	}
	if value := strings.TrimSpace(match.Overview); value != "" {
		updates["overview"] = value
	}
	if value := strings.TrimSpace(match.PosterURL); value != "" {
		updates["poster_url"] = value
	}
	if value := strings.TrimSpace(match.BackdropURL); value != "" {
		updates["backdrop_url"] = value
	}
	if match.Rating > 0 {
		updates["rating"] = match.Rating
	}
	if match.Year > 0 {
		updates["year"] = match.Year
	}
	if value := strings.TrimSpace(match.ReleaseDate); value != "" {
		updates["release_date"] = value
	}
	if match.TMDbID > 0 {
		updates["tm_db_id"] = match.TMDbID
	}
	if match.BangumiID > 0 {
		updates["bangumi_id"] = match.BangumiID
	}
	if value := strings.TrimSpace(match.DoubanID); value != "" {
		updates["douban_id"] = value
	}
	if value := strings.TrimSpace(match.TheTVDBID); value != "" {
		updates["thetvdb_id"] = value
	}
	if len(match.Genres) > 0 {
		updates["genres"] = strings.Join(match.Genres, ",")
	}
	if len(match.Countries) > 0 {
		updates["countries"] = strings.Join(match.Countries, ",")
	}
	if len(match.Languages) > 0 {
		updates["languages"] = strings.Join(match.Languages, ",")
	}
	if match.NSFW {
		updates["nsfw"] = true
	}
	updates["scrape_status"] = "matched"
}

func (o *OrganizerService) deleteMediaRowForPath(ctx context.Context, path string) {
	if o == nil || o.repo == nil || o.repo.DB == nil {
		return
	}
	_ = o.repo.DB.WithContext(ctx).Where("path = ?", path).Delete(&model.Media{}).Error
}

func (o *OrganizerService) mediaPathExists(ctx context.Context, path string) bool {
	if o == nil || o.repo == nil || o.repo.DB == nil {
		return false
	}
	var count int64
	if err := o.repo.DB.WithContext(ctx).Unscoped().Model(&model.Media{}).Where("path = ?", path).Count(&count).Error; err != nil {
		return false
	}
	return count > 0
}
