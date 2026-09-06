package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// UpdateWithCurrentSeriesKey applies media fields and, when a grouping input
// changed, restores the authoritative persisted series key in the same
// transaction. PostgreSQL/SQLite triggers deliberately invalidate the key on
// direct metadata writes; this method closes that dirty window for known write
// paths instead of waiting for background maintenance or a read fallback.
//
// Pass tx when this update is part of a wider transaction. Pass nil to have the
// repository create the required transaction itself.
func (r *MediaRepository) UpdateWithCurrentSeriesKey(ctx context.Context, tx *gorm.DB, mediaID string, updates map[string]any) error {
	if r == nil || r.db == nil {
		return errors.New("media repository unavailable")
	}
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return errors.New("media id required")
	}
	if len(updates) == 0 {
		return nil
	}

	write := func(db *gorm.DB) error {
		result := db.WithContext(ctx).Model(&model.Media{}).Where("id = ?", mediaID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		if !mediaSeriesKeyInputsChanged(updates) {
			return nil
		}

		var updated model.Media
		if err := db.WithContext(ctx).Where("id = ?", mediaID).First(&updated).Error; err != nil {
			return err
		}
		r.PrepareSeriesKey(&updated)
		if strings.TrimSpace(updated.SeriesKey) == "" || updated.SeriesKeyVersion != mediaSeriesKeyVersion {
			return errors.New("updated media has no current series key")
		}
		return db.WithContext(ctx).Model(&model.Media{}).Where("id = ?", mediaID).
			UpdateColumns(map[string]any{
				"series_key":         updated.SeriesKey,
				"series_key_version": updated.SeriesKeyVersion,
			}).Error
	}

	var err error
	if tx != nil {
		err = write(tx)
	} else if mediaSeriesKeyInputsChanged(updates) {
		err = r.db.WithContext(ctx).Transaction(write)
	} else {
		err = write(r.db)
	}
	return err
}

// UpdateManyWithCurrentSeriesKeys is the batch counterpart used by episode
// propagation and other ingest-time group updates. It applies one SQL update,
// then restores each affected row's authoritative physical series key inside
// the same transaction.
func (r *MediaRepository) UpdateManyWithCurrentSeriesKeys(ctx context.Context, tx *gorm.DB, mediaIDs []string, updates map[string]any) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("media repository unavailable")
	}
	ids := make([]string, 0, len(mediaIDs))
	seen := make(map[string]struct{}, len(mediaIDs))
	for _, id := range mediaIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 || len(updates) == 0 {
		return 0, nil
	}

	var affected int64
	write := func(db *gorm.DB) error {
		result := db.WithContext(ctx).Model(&model.Media{}).Where("id IN ?", ids).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		affected = result.RowsAffected
		if !mediaSeriesKeyInputsChanged(updates) {
			return nil
		}

		var rows []model.Media
		if err := db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			return fmt.Errorf("update media series keys: found %d of %d active rows", len(rows), len(ids))
		}
		for i := range rows {
			r.PrepareSeriesKey(&rows[i])
			if strings.TrimSpace(rows[i].SeriesKey) == "" || rows[i].SeriesKeyVersion != mediaSeriesKeyVersion {
				return fmt.Errorf("updated media %q has no current series key", rows[i].ID)
			}
			if err := db.WithContext(ctx).Model(&model.Media{}).Where("id = ?", rows[i].ID).
				UpdateColumns(map[string]any{
					"series_key":         rows[i].SeriesKey,
					"series_key_version": rows[i].SeriesKeyVersion,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	}

	var err error
	if tx != nil {
		err = write(tx)
	} else if mediaSeriesKeyInputsChanged(updates) {
		err = r.db.WithContext(ctx).Transaction(write)
	} else {
		err = write(r.db)
	}
	return affected, err
}
