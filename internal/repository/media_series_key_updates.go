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
		seriesKeyChanged := mediaSeriesKeyInputsChanged(updates)
		versionKeyChanged := r.versionKeyFunc != nil && mediaVersionKeyInputsChanged(updates)
		if !seriesKeyChanged && !versionKeyChanged {
			return nil
		}

		var updated model.Media
		if err := db.WithContext(ctx).Where("id = ?", mediaID).First(&updated).Error; err != nil {
			return err
		}
		columns := map[string]any{}
		if seriesKeyChanged {
			r.PrepareSeriesKey(&updated)
		}
		if seriesKeyChanged && (strings.TrimSpace(updated.SeriesKey) == "" || updated.SeriesKeyVersion != mediaSeriesKeyVersion) {
			return errors.New("updated media has no current series key")
		}
		if versionKeyChanged {
			r.PrepareVersionKey(&updated)
			if strings.TrimSpace(updated.MediaVersionKey) == "" || updated.MediaVersionKeyVersion != mediaVersionKeyVersion {
				return errors.New("updated media has no current version key")
			}
		}
		if seriesKeyChanged {
			columns["series_key"] = updated.SeriesKey
			columns["series_key_version"] = updated.SeriesKeyVersion
		}
		if versionKeyChanged {
			columns["media_version_key"] = updated.MediaVersionKey
			columns["media_version_key_version"] = updated.MediaVersionKeyVersion
		}
		return db.WithContext(ctx).Model(&model.Media{}).Where("id = ?", mediaID).UpdateColumns(columns).Error
	}

	var err error
	if tx != nil {
		err = write(tx)
	} else if mediaSeriesKeyInputsChanged(updates) || (r.versionKeyFunc != nil && mediaVersionKeyInputsChanged(updates)) {
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
		seriesKeyChanged := mediaSeriesKeyInputsChanged(updates)
		versionKeyChanged := r.versionKeyFunc != nil && mediaVersionKeyInputsChanged(updates)
		if !seriesKeyChanged && !versionKeyChanged {
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
			columns := map[string]any{}
			if seriesKeyChanged {
				r.PrepareSeriesKey(&rows[i])
			}
			if seriesKeyChanged && (strings.TrimSpace(rows[i].SeriesKey) == "" || rows[i].SeriesKeyVersion != mediaSeriesKeyVersion) {
				return fmt.Errorf("updated media %q has no current series key", rows[i].ID)
			}
			if versionKeyChanged {
				r.PrepareVersionKey(&rows[i])
				if strings.TrimSpace(rows[i].MediaVersionKey) == "" || rows[i].MediaVersionKeyVersion != mediaVersionKeyVersion {
					return fmt.Errorf("updated media %q has no current version key", rows[i].ID)
				}
			}
			if seriesKeyChanged {
				columns["series_key"] = rows[i].SeriesKey
				columns["series_key_version"] = rows[i].SeriesKeyVersion
			}
			if versionKeyChanged {
				columns["media_version_key"] = rows[i].MediaVersionKey
				columns["media_version_key_version"] = rows[i].MediaVersionKeyVersion
			}
			if err := db.WithContext(ctx).Model(&model.Media{}).Where("id = ?", rows[i].ID).UpdateColumns(columns).Error; err != nil {
				return err
			}
		}
		return nil
	}

	var err error
	if tx != nil {
		err = write(tx)
	} else if mediaSeriesKeyInputsChanged(updates) || (r.versionKeyFunc != nil && mediaVersionKeyInputsChanged(updates)) {
		err = r.db.WithContext(ctx).Transaction(write)
	} else {
		err = write(r.db)
	}
	return affected, err
}
