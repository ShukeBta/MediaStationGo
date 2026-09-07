package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const EmbyKeyVersion = 1

func (r *MediaRepository) SetEmbyKeyFunc(fn func(*model.Media), configKey func() string) {
	r.embyKeyFunc = fn
	r.embyConfigKeyFunc = configKey
}

func (r *MediaRepository) PrepareEmbyKeys(row *model.Media) {
	if r.embyKeyFunc == nil || row == nil {
		return
	}
	r.embyKeyFunc(row)
	row.EmbyKeyVersion = EmbyKeyVersion
}

func (r *MediaRepository) embyProjectionStale(row model.Media) bool {
	return r.embyKeyFunc != nil && (row.EmbyKeyVersion != EmbyKeyVersion || row.EmbySeriesKey == "" || row.EmbyListKey == "" || row.EmbyConfigKey != r.embyConfigKeyFunc())
}

func mediaEmbyKeyInputsChanged(updates map[string]any) bool {
	for _, column := range []string{"library_id", "series_id", "title", "original_name", "path", "part_group_key", "release_date", "relative_path", "genres", "languages", "countries", "nsfw"} {
		if _, ok := updates[column]; ok {
			return true
		}
	}
	return false
}

// RefreshEmbyKeys must use the caller's transaction after metadata updates.
// It also handles restored soft-deleted rows without changing their timestamps.
func (r *MediaRepository) RefreshEmbyKeys(ctx context.Context, tx *gorm.DB, ids []string) error {
	if r.embyKeyFunc == nil || len(ids) == 0 {
		return nil
	}
	var rows []model.Media
	q := tx.WithContext(ctx).Unscoped().Select("id", "library_id", "series_id", "title", "original_name", "path", "part_group_key", "release_date", "relative_path", "genres", "languages", "countries", "nsfw").Where("id IN ?", ids)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if err := q.Find(&rows).Error; err != nil {
		return err
	}
	updates := make([]map[string]any, 0, len(rows))
	for i := range rows {
		r.PrepareEmbyKeys(&rows[i])
		if rows[i].EmbySeriesKey == "" || rows[i].EmbyListKey == "" {
			return errors.New("empty Emby grouping identity")
		}
		updates = append(updates, map[string]any{
			"id":              rows[i].ID,
			"emby_series_key": rows[i].EmbySeriesKey, "emby_list_key": rows[i].EmbyListKey, "emby_key_version": EmbyKeyVersion,
			"emby_series_name": rows[i].EmbySeriesName, "emby_premiere_date": rows[i].EmbyPremiereDate, "emby_genres": rows[i].EmbyGenres, "emby_config_key": rows[i].EmbyConfigKey,
			"emby_genre_variants": rows[i].EmbyGenreVariants,
		})
	}
	if len(updates) == 0 {
		return nil
	}
	if tx.Dialector.Name() == "postgres" {
		payload, err := json.Marshal(updates)
		if err != nil {
			return err
		}
		return tx.WithContext(ctx).Exec(`UPDATE media AS target SET
emby_series_key = source.emby_series_key, emby_list_key = source.emby_list_key,
emby_key_version = source.emby_key_version, emby_series_name = source.emby_series_name,
emby_premiere_date = source.emby_premiere_date, emby_genres = source.emby_genres,
emby_genre_variants = source.emby_genre_variants, emby_config_key = source.emby_config_key
FROM jsonb_to_recordset(CAST(? AS jsonb)) AS source(
id text, emby_series_key text, emby_list_key text, emby_key_version integer,
emby_series_name text, emby_premiere_date text, emby_genres text,
emby_genre_variants text, emby_config_key text)
WHERE target.id = source.id`, string(payload)).Error
	}
	for _, update := range updates {
		id := update["id"]
		delete(update, "id")
		if err := tx.WithContext(ctx).Unscoped().Model(&model.Media{}).Where("id = ?", id).UpdateColumns(update).Error; err != nil {
			return err
		}
	}
	return nil
}

// BackfillEmbyKeys is an explicit, bounded migration/repair operation. Rows are
// locked before calculation, so a concurrent move cannot publish an old key.
func (r *MediaRepository) BackfillEmbyKeys(ctx context.Context, limit int) (int64, error) {
	if r.embyKeyFunc == nil {
		return 0, errors.New("Emby identity calculator unavailable")
	}
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []model.Media
		q := tx.Select("id").Where("emby_key_version <> ? OR emby_series_key IS NULL OR emby_series_key = '' OR emby_list_key IS NULL OR emby_list_key = '' OR COALESCE(emby_config_key, '') <> ?", EmbyKeyVersion, r.embyConfigKeyFunc()).Order("id ASC").Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Find(&rows).Error; err != nil {
			return err
		}
		ids := make([]string, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		if err := r.RefreshEmbyKeys(ctx, tx, ids); err != nil {
			return err
		}
		count = int64(len(ids))
		return nil
	})
	return count, err
}
