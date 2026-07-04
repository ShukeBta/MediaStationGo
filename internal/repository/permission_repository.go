package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// PermissionRepository persists model.UserPermission records.
type PermissionRepository struct{ db *gorm.DB }

// Create inserts a new permission record.
func (r *PermissionRepository) Create(ctx context.Context, p *model.UserPermission) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Create(p).Error
	})
}

// FindByUserID returns the permission record for a user, or (nil, nil) when absent.
func (r *PermissionRepository) FindByUserID(ctx context.Context, userID string) (*model.UserPermission, error) {
	var p model.UserPermission
	err := withSQLiteBusyRetry(ctx, func() error {
		p = model.UserPermission{}
		return r.db.WithContext(ctx).Where("user_id = ?", userID).First(&p).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Update updates permission fields for a user.
func (r *PermissionRepository) Update(ctx context.Context, userID string, updates map[string]bool) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Model(&model.UserPermission{}).
			Where("user_id = ?", userID).Updates(updates).Error
	})
}

// Upsert creates or updates a permission record.
func (r *PermissionRepository) Upsert(ctx context.Context, p *model.UserPermission) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Where("user_id = ?", p.UserID).
			Assign(*p).FirstOrCreate(p).Error
	})
}

// Delete removes a permission record.
func (r *PermissionRepository) Delete(ctx context.Context, userID string) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.UserPermission{}).Error
	})
}
