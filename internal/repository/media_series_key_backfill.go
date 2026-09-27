package repository

import (
	"context"
	"errors"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

// BackfillSeriesKeys computes current grouping keys in bounded batches. It is
// intentionally incremental so startup never holds the media table or a large
// transaction for the duration of a production migration.
func (r *MediaRepository) BackfillSeriesKeys(ctx context.Context, batchLimit int) (int64, error) {
	return r.BackfillSeriesKeysFiltered(ctx, nil, MediaQueryFilter{IncludeNSFW: true}, batchLimit)
}

// BackfillSeriesKeysFiltered repairs only the visibility/library scope that
// blocked a persisted series query. This keeps request-time self-repair
// bounded and prevents unrelated stale rows from consuming the whole batch.
func (r *MediaRepository) BackfillSeriesKeysFiltered(ctx context.Context, libraryIDs []string, filter MediaQueryFilter, batchLimit int) (int64, error) {
	if r == nil || r.db == nil || r.seriesKeyFunc == nil {
		return 0, nil
	}
	if batchLimit <= 0 {
		batchLimit = 500
	}
	if batchLimit > 5000 {
		batchLimit = 5000
	}
	var rows []model.Media
	query := r.db.WithContext(ctx).Where("deleted_at IS NULL AND (series_key_version <> ? OR series_key_version IS NULL OR series_key IS NULL OR series_key = '')", mediaSeriesKeyVersion)
	if len(libraryIDs) == 1 {
		query = query.Where("library_id = ?", libraryIDs[0])
	} else if len(libraryIDs) > 1 {
		query = query.Where("library_id IN ?", libraryIDs)
	}
	query = applyMediaQueryFilter(query, filter)
	if err := query.Order("id ASC").Limit(batchLimit).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	updates := make([]map[string]any, 0, len(rows))
	for i := range rows {
		key := r.seriesKeyFunc(rows[i])
		if key == "" {
			updates = append(updates, map[string]any{"id": rows[i].ID, "series_key": "", "series_key_version": 0})
			continue
		}
		updates = append(updates, map[string]any{"id": rows[i].ID, "series_key": key, "series_key_version": mediaSeriesKeyVersion})
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			id, ok := update["id"].(string)
			if !ok || id == "" {
				return errors.New("series key backfill row missing id")
			}
			if err := tx.Model(&model.Media{}).Where("id = ?", id).Updates(update).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return int64(len(rows)), nil
}
