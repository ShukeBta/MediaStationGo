package repository

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

// inheritManualSeriesGroup adopts only an unambiguous, same-library work.
// Conflicting provider IDs and explicit user assignments are never replaced.
func (r *MediaRepository) inheritManualSeriesGroup(ctx context.Context, db *gorm.DB, row *model.Media) (bool, error) {
	if row.LibraryID == "" || row.PartGroupKey != "" || (row.SeasonNum <= 0 && row.EpisodeNum <= 0 && row.SeriesID == "") {
		return false, nil
	}
	identity := db.Where("1 = 0")
	for column, value := range map[string]any{"tm_db_id": row.TMDbID, "bangumi_id": row.BangumiID, "douban_id": row.DoubanID, "thetvdb_id": row.TheTVDBID, "series_id": row.SeriesID} {
		if value != 0 && value != "" {
			identity = identity.Or(column+" = ?", value)
		}
	}
	if row.ScrapeStatus == "matched" && row.Year > 0 && strings.TrimSpace(row.Title) != "" {
		identity = identity.Or("scrape_status = 'matched' AND year = ? AND LOWER(TRIM(title)) = ?", row.Year, strings.ToLower(strings.TrimSpace(row.Title)))
	}
	var candidates []model.Media
	if err := db.WithContext(ctx).Where("library_id = ? AND part_group_key <> '' AND part_group_key NOT LIKE 'auto-part:%'", row.LibraryID).
		Where(identity).Find(&candidates).Error; err != nil {
		return false, err
	}
	var selected *model.Media
	for i := range candidates {
		candidate := &candidates[i]
		if candidate.SeasonNum <= 0 && candidate.EpisodeNum <= 0 && candidate.SeriesID == "" {
			continue
		}
		if (row.TMDbID > 0 && candidate.TMDbID > 0 && row.TMDbID != candidate.TMDbID) ||
			(row.BangumiID > 0 && candidate.BangumiID > 0 && row.BangumiID != candidate.BangumiID) ||
			(row.DoubanID != "" && candidate.DoubanID != "" && row.DoubanID != candidate.DoubanID) ||
			(row.TheTVDBID != "" && candidate.TheTVDBID != "" && row.TheTVDBID != candidate.TheTVDBID) {
			continue
		}
		if selected != nil && selected.PartGroupKey != candidate.PartGroupKey {
			return false, nil
		}
		selected = candidate
	}
	if selected == nil {
		return false, nil
	}
	var last int
	if err := db.WithContext(ctx).Model(&model.Media{}).Where("library_id = ? AND part_group_key = ?", row.LibraryID, selected.PartGroupKey).
		Select("COALESCE(MAX(part_index), 0)").Scan(&last).Error; err != nil {
		return false, err
	}
	row.PartGroupKey, row.PartGroupTitle, row.PartIndex = selected.PartGroupKey, selected.PartGroupTitle, last+1
	return true, nil
}

func (r *MediaRepository) inheritStoredManualSeriesGroup(ctx context.Context, db *gorm.DB, row *model.Media) error {
	changed, err := r.inheritManualSeriesGroup(ctx, db, row)
	if err != nil || !changed {
		return err
	}
	if err := db.WithContext(ctx).Model(&model.Media{}).Where("id = ?", row.ID).Updates(map[string]any{
		"part_group_key": row.PartGroupKey, "part_group_title": row.PartGroupTitle, "part_index": row.PartIndex,
	}).Error; err != nil {
		return err
	}
	return r.RefreshEmbyKeys(ctx, db, []string{row.ID})
}

func mediaAwaitingSeriesIdentity(row model.Media) bool {
	return row.PartGroupKey == "" && row.SeriesID == "" && row.TMDbID == 0 && row.BangumiID == 0 && row.DoubanID == "" && row.TheTVDBID == "" && row.ScrapeStatus != "matched"
}

func pendingManualSeriesCandidates(ctx context.Context, db *gorm.DB, ids []string, updates map[string]any) (map[string]bool, error) {
	eligible := make(map[string]bool)
	if _, explicit := updates["part_group_key"]; explicit || !mediaSeriesKeyInputsChanged(updates) {
		return eligible, nil
	}
	var before []model.Media
	if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&before).Error; err != nil {
		return nil, err
	}
	for _, row := range before {
		eligible[row.ID] = mediaAwaitingSeriesIdentity(row)
	}
	return eligible, nil
}
