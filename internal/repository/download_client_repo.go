// Package repository 实现下载客户端配置的数据访问层。
package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// DownloadClientRepository persists model.DownloadClient records.
type DownloadClientRepository struct{ db *gorm.DB }

// Create inserts a new download client.
func (r *DownloadClientRepository) Create(ctx context.Context, c *model.DownloadClient) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// FindByID returns the download client by ID, or (nil, nil) when absent.
func (r *DownloadClientRepository) FindByID(ctx context.Context, id string) (*model.DownloadClient, error) {
	var c model.DownloadClient
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// FindDefault returns the default download client, or (nil, nil).
func (r *DownloadClientRepository) FindDefault(ctx context.Context) (*model.DownloadClient, error) {
	var c model.DownloadClient
	err := r.db.WithContext(ctx).
		Where("is_default = ? AND enabled = ?", true, true).
		Order("created_at asc").
		First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List returns all download clients ordered by creation time.
func (r *DownloadClientRepository) List(ctx context.Context) ([]model.DownloadClient, error) {
	var rows []model.DownloadClient
	err := r.db.WithContext(ctx).Order("created_at asc").Find(&rows).Error
	return rows, err
}

// ListEnabled returns all enabled download clients.
func (r *DownloadClientRepository) ListEnabled(ctx context.Context) ([]model.DownloadClient, error) {
	var rows []model.DownloadClient
	err := r.db.WithContext(ctx).Where("enabled = ?", true).Order("created_at asc").Find(&rows).Error
	return rows, err
}

// HasAnyIncludingDeleted reports whether the operator has ever configured a
// download client. This distinguishes legacy-only installations from systems
// where deleting/disabling all clients is an intentional "stop downloads"
// action, even though rows are soft-deleted.
func (r *DownloadClientRepository) HasAnyIncludingDeleted(ctx context.Context) (bool, error) {
	var n int64
	err := r.db.WithContext(ctx).Unscoped().Model(&model.DownloadClient{}).Count(&n).Error
	return n > 0, err
}

// Update persists changes to a download client.
func (r *DownloadClientRepository) Update(ctx context.Context, c *model.DownloadClient) error {
	return r.db.WithContext(ctx).Save(c).Error
}

// Delete removes a download client (soft-delete).
func (r *DownloadClientRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.DownloadClient{}, "id = ?", id).Error
}

// ClearDefault unsets the default flag for all clients.
func (r *DownloadClientRepository) ClearDefault(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&model.DownloadClient{}).
		Where("is_default = ?", true).Update("is_default", false).Error
}

// SetDefault sets a specific client as default and clears others.
func (r *DownloadClientRepository) SetDefault(ctx context.Context, id string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.DownloadClient{}).
			Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		return tx.Model(&model.DownloadClient{}).
			Where("id = ?", id).Updates(map[string]any{
			"is_default": true,
			"updated_at": now,
		}).Error
	})
}
