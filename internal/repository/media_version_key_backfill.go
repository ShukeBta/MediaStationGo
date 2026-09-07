package repository

import (
	"context"
	"errors"
	"fmt"
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
	for i := range rows {
		r.PrepareVersionKey(&rows[i])
	}
	if r.db.Dialector.Name() == "postgres" {
		values := make([]string, 0, len(rows))
		args := make([]any, 0, len(rows)*3)
		for i := range rows {
			values = append(values, "(?, ?, ?)")
			args = append(args, rows[i].ID, rows[i].MediaVersionKey, rows[i].MediaVersionKeyVersion)
		}
		query := fmt.Sprintf(`
UPDATE media AS target
SET media_version_key = source.media_version_key,
    media_version_key_version = source.media_version_key_version
FROM (VALUES %s) AS source(id, media_version_key, media_version_key_version)
WHERE target.id = source.id`, strings.Join(values, ","))
		if err := r.db.WithContext(ctx).Exec(query, args...).Error; err != nil {
			return 0, err
		}
		return int64(len(rows)), nil
	}

	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for i := range rows {
			if err := tx.Model(&model.Media{}).Where("id = ?", strings.TrimSpace(rows[i].ID)).UpdateColumns(map[string]any{
				"media_version_key":         rows[i].MediaVersionKey,
				"media_version_key_version": rows[i].MediaVersionKeyVersion,
			}).Error; err != nil {
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
