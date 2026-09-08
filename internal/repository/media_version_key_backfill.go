package repository

import (
	"context"
	"encoding/json"
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
	maxBatch := 1000
	if r.db.Dialector.Name() == "postgres" {
		maxBatch = 10000
	}
	if batchLimit > maxBatch {
		batchLimit = maxBatch
	}
	q := r.db.WithContext(ctx).Where("deleted_at IS NULL AND (media_version_key_version <> ? OR media_version_key_version IS NULL OR media_version_key IS NULL OR media_version_key = '')", mediaVersionKeyVersion)
	if len(libraryIDs) > 0 {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	var rows []model.Media
	if err := q.Select(
		"id", "library_id", "title", "original_name", "path",
		"part_group_key", "part_index", "version_group_key", "title_cleanup_version",
		"season_num", "episode_num", "episode_end_num", "episode_part_num", "year", "tm_db_id", "bangumi_id", "douban_id", "thetvdb_id",
	).Order("created_at ASC, id ASC").Limit(batchLimit).Find(&rows).Error; err != nil {
		return 0, err
	}
	if len(rows) == 0 {
		return 0, nil
	}
	for i := range rows {
		r.PrepareVersionKey(&rows[i])
	}
	if r.db.Dialector.Name() == "postgres" {
		type versionKeyUpdate struct {
			ID      string `json:"id"`
			Key     string `json:"media_version_key"`
			Version int    `json:"media_version_key_version"`
		}
		updates := make([]versionKeyUpdate, 0, len(rows))
		for i := range rows {
			updates = append(updates, versionKeyUpdate{
				ID:      rows[i].ID,
				Key:     rows[i].MediaVersionKey,
				Version: rows[i].MediaVersionKeyVersion,
			})
		}
		payload, err := json.Marshal(updates)
		if err != nil {
			return 0, err
		}
		const query = `
UPDATE media AS target
SET media_version_key = source.media_version_key,
    media_version_key_version = source.media_version_key_version
FROM jsonb_to_recordset(CAST(? AS jsonb)) AS source(
  id varchar,
  media_version_key varchar,
  media_version_key_version bigint
)
WHERE target.id = source.id`
		if err := r.db.WithContext(ctx).Exec(query, string(payload)).Error; err != nil {
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
