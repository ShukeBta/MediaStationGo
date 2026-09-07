package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// BackfillMediaVersionKeys repairs the persisted effective grouping key in
// bounded batches. The service-owned calculator remains the single source of
// truth; this repository only persists its result.
func (r *MediaRepository) BackfillMediaVersionKeys(ctx context.Context, batchLimit int) (int64, error) {
	return r.BackfillMediaVersionKeysFiltered(ctx, nil, MediaQueryFilter{IncludeNSFW: true}, batchLimit)
}

func (r *MediaRepository) BackfillMediaVersionKeysFiltered(ctx context.Context, libraryIDs []string, filter MediaQueryFilter, batchLimit int) (int64, error) {
	if r == nil || r.db == nil || r.versionKeyFunc == nil {
		return 0, errors.New("media version key calculator unavailable")
	}
	if batchLimit <= 0 {
		batchLimit = 100
	}
	if batchLimit > 1000 {
		batchLimit = 1000
	}
	q := r.db.WithContext(ctx).Where("deleted_at IS NULL AND (media_version_key_version <> ? OR media_version_key_version IS NULL OR media_version_key IS NULL OR media_version_key = '')", mediaVersionKeyVersion)
	if len(libraryIDs) > 0 {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	var rows []model.Media
	if err := q.Order("created_at ASC, id ASC").Limit(batchLimit).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	updates := make([]map[string]any, 0, len(rows))
	for i := range rows {
		r.PrepareVersionKey(&rows[i])
		updates = append(updates, map[string]any{
			"id":                        rows[i].ID,
			"media_version_key":         rows[i].MediaVersionKey,
			"media_version_key_version": rows[i].MediaVersionKeyVersion,
		})
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, update := range updates {
			id, _ := update["id"].(string)
			delete(update, "id")
			if err := tx.Model(&model.Media{}).Where("id = ?", strings.TrimSpace(id)).UpdateColumns(update).Error; err != nil {
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
