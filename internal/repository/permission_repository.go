package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// PermissionRepository persists model.UserPermission records.
type PermissionRepository struct{ db *gorm.DB }

var permissionWriteFields = []string{
	"ID",
	"UserID",
	"CanViewDashboard",
	"CanPlayMedia",
	"CanCast",
	"CanExternalPlayer",
	"CanFavorite",
	"CanViewHistory",
	"CanEditMedia",
	"CanRescrape",
	"CanUseAI",
	"CanCaptureFrames",
	"CanManageDownloads",
	"CanViewDiscover",
	"CanManageSubscriptions",
	"CanManageSites",
	"CanUseAIAssistant",
	"CanManageUsers",
	"CanManageFiles",
	"CanManageStrm",
	"CanAccessSettings",
}

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
		db := r.db.WithContext(ctx)
		var existing model.UserPermission
		err := db.Where("user_id = ?", p.UserID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// A struct insert applies GORM's default:true to explicit false
			// values. A map preserves every administrator-selected permission.
			if err := p.BeforeCreate(db); err != nil {
				return err
			}
			values := make(map[string]any)
			for key, value := range p.PermissionMap() {
				values[key] = value
			}
			values["id"], values["user_id"] = p.ID, p.UserID
			values["created_at"], values["updated_at"] = time.Now(), time.Now()
			return db.Model(&model.UserPermission{}).Create(values).Error
		}
		if err != nil {
			return err
		}
		p.ID = existing.ID
		return db.Model(&existing).Select(permissionWriteFields[2:]).Updates(p).Error
	})
}

// Delete removes a permission record.
func (r *PermissionRepository) Delete(ctx context.Context, userID string) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&model.UserPermission{}).Error
	})
}
