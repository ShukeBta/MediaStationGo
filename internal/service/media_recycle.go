package service

import (
	"context"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const maxRecycleBinRecords = 200

// SoftDelete moves a media row to the recycle bin (gorm soft delete).
// The on-disk file is kept; admins can purge it later.
func (s *MediaService) SoftDelete(ctx context.Context, id string) error {
	media, err := s.repo.Media.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if media != nil && isCloudMediaPath(media.Path) {
		err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&model.Media{}).Error
		if err == nil {
			s.invalidateMediaCache(ctx)
		}
		return err
	}
	err = s.repo.DB.WithContext(ctx).Where("id = ?", id).Delete(&model.Media{}).Error
	if err == nil {
		if pruneErr := pruneRecycleBinRows(ctx, s.repo.DB, maxRecycleBinRecords); pruneErr != nil {
			return pruneErr
		}
		s.invalidateMediaCache(ctx)
	}
	return err
}

// RestoreDeleted unsets DeletedAt for a single media row.
func (s *MediaService) RestoreDeleted(ctx context.Context, id string) error {
	err := s.repo.DB.WithContext(ctx).Unscoped().Model(&model.Media{}).
		Where("id = ?", id).Update("deleted_at", nil).Error
	if err == nil {
		s.invalidateMediaCache(ctx)
	}
	return err
}

// ListRecycleBin returns every soft-deleted row, newest first.
func (s *MediaService) ListRecycleBin(ctx context.Context, limit int) ([]model.Media, error) {
	if err := pruneRecycleBinRows(ctx, s.repo.DB, maxRecycleBinRecords); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxRecycleBinRecords {
		limit = maxRecycleBinRecords
	}
	var rows []model.Media
	err := s.repo.DB.Unscoped().
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Limit(limit).
		Find(&rows).Error
	return rows, err
}

func pruneRecycleBinRows(ctx context.Context, db *gorm.DB, keep int) error {
	if db == nil {
		return nil
	}
	if keep <= 0 {
		keep = maxRecycleBinRecords
	}
	var rows []struct {
		ID string
	}
	if err := db.WithContext(ctx).Unscoped().
		Model(&model.Media{}).
		Select("id").
		Where("deleted_at IS NOT NULL").
		Order("deleted_at desc").
		Limit(100000).
		Offset(keep).
		Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.ID != "" {
			ids = append(ids, row.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	return db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Delete(&model.Media{}).Error
}

// PurgeDeleted permanently removes a soft-deleted row from the database.
func (s *MediaService) PurgeDeleted(ctx context.Context, id string) error {
	err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).Delete(&model.Media{}).Error
	if err == nil {
		s.invalidateMediaCache(ctx)
	}
	return err
}
