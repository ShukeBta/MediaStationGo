package service

import (
	"context"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// RepairCloudPathMetadata backfills external IDs from media paths such as
// "Movie (2025) {tmdb-123}" so existing placeholder rows can be scraped
// without requiring another successful filesystem or cloud provider traversal.
//
// 传入 libraryID 时只修复这些媒体库的行;为空则修复全库。
func (c *Container) RepairCloudPathMetadata(ctx context.Context, libraryID ...string) (int, error) {
	if c == nil || c.Repo == nil || c.Repo.DB == nil {
		return 0, nil
	}
	libraryIDs := compactLibraryIDs(libraryID...)
	var repaired int
	var rows []model.Media
	query := c.Repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("id, title, path, year, season_num, episode_num, scrape_status, tm_db_id, bangumi_id, douban_id, thetvdb_id").
		Where("("+strings.Join([]string{
			"LOWER(path) LIKE ?",
			"LOWER(path) LIKE ?",
			"LOWER(path) LIKE ?",
			"LOWER(path) LIKE ?",
			"LOWER(path) LIKE ?",
			"LOWER(path) LIKE ?",
			"LOWER(path) LIKE ?",
			"LOWER(path) LIKE ?",
		}, " OR ")+")",
			"%tmdb%", "%tmdbid%", "%douban%", "%db%", "%bangumi%", "%bgm%", "%thetvdb%", "%tvdb%")
	if len(libraryIDs) > 0 {
		query = query.Where("library_id IN ?", libraryIDs)
	}

	err := query.FindInBatches(&rows, 500, func(_ *gorm.DB, _ int) error {
		for _, row := range rows {
			meta, hints := pathHintMetadata(row.Path, row.SeasonNum > 0 || row.EpisodeNum > 0)
			if meta == nil || !hints.useful() {
				continue
			}
			updates := map[string]any{}
			status := strings.TrimSpace(row.ScrapeStatus)
			enrichable := status == "" || status == "pending" || status == "no_match"
			changedExternalID := false
			if meta.TMDbID > 0 && row.TMDbID != meta.TMDbID {
				updates["tm_db_id"] = meta.TMDbID
				changedExternalID = true
			}
			if meta.BangumiID > 0 && row.BangumiID != meta.BangumiID {
				updates["bangumi_id"] = meta.BangumiID
				changedExternalID = true
			}
			if strings.TrimSpace(meta.DoubanID) != "" && strings.TrimSpace(row.DoubanID) != strings.TrimSpace(meta.DoubanID) {
				updates["douban_id"] = strings.TrimSpace(meta.DoubanID)
				changedExternalID = true
			}
			if strings.TrimSpace(meta.TheTVDBID) != "" && strings.TrimSpace(row.TheTVDBID) != strings.TrimSpace(meta.TheTVDBID) {
				updates["thetvdb_id"] = strings.TrimSpace(meta.TheTVDBID)
				changedExternalID = true
			}
			if meta.Year > 0 && row.Year <= 0 {
				updates["year"] = meta.Year
			}
			if enrichable && strings.TrimSpace(meta.Title) != "" && cloudPathRepairShouldReplaceTitle(row.Title, meta.Title) {
				updates["title"] = strings.TrimSpace(meta.Title)
			}
			if changedExternalID && (status == "" || status == "no_match" || status == "matched") {
				updates["scrape_status"] = "pending"
			}
			if len(updates) == 0 {
				continue
			}
			if err := c.Repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return err
			}
			repaired++
		}
		return nil
	}).Error
	if err != nil {
		return repaired, err
	}
	if repaired > 0 && c.Log != nil {
		c.Log.Info("cloud path metadata repaired", zap.Int("media_count", repaired))
	}
	return repaired, nil
}

func cloudPathRepairShouldReplaceTitle(current, hinted string) bool {
	current = strings.TrimSpace(current)
	hinted = strings.TrimSpace(hinted)
	if hinted == "" || strings.EqualFold(current, hinted) {
		return false
	}
	if current == "" {
		return true
	}
	noise := []string{"web-dl", "bluray", "hdtv", "2160p", "1080p", "720p", "ddp", "aac", "h.264", "h.265", "x264", "x265", "adweb", "mweb", "cmctv", "bit"}
	lower := strings.ToLower(current)
	for _, token := range noise {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return len([]rune(current)) > len([]rune(hinted))*2
}

func compactLibraryIDs(ids ...string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = appendUniqueLibraryIDs(out, id)
	}
	return out
}
