package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ApiConfigRepository persists model.ApiConfig records.
type ApiConfigRepository struct{ db *gorm.DB }

// Create inserts a new API config record.
func (r *ApiConfigRepository) Create(ctx context.Context, c *model.ApiConfig) error {
	return r.db.WithContext(ctx).Create(c).Error
}

// FindByProvider returns the API config for a provider, or (nil, nil).
func (r *ApiConfigRepository) FindByProvider(ctx context.Context, provider string) (*model.ApiConfig, error) {
	var c model.ApiConfig
	err := r.db.WithContext(ctx).Where("provider = ?", provider).First(&c).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// List returns all API configs.
func (r *ApiConfigRepository) List(ctx context.Context) ([]model.ApiConfig, error) {
	var rows []model.ApiConfig
	err := r.db.WithContext(ctx).Order("provider asc").Find(&rows).Error
	return rows, err
}

// Upsert creates or updates an API config.
func (r *ApiConfigRepository) Upsert(ctx context.Context, c *model.ApiConfig) error {
	return r.db.WithContext(ctx).Where("provider = ?", c.Provider).
		Assign(model.ApiConfig{
			Base:    model.Base{UpdatedAt: time.Now()},
			APIKey:  c.APIKey,
			BaseURL: c.BaseURL,
			Extra:   c.Extra,
			Enabled: c.Enabled,
		}).FirstOrCreate(c).Error
}

// Update updates an API config.
func (r *ApiConfigRepository) Update(ctx context.Context, c *model.ApiConfig) error {
	return r.db.WithContext(ctx).Model(&model.ApiConfig{}).
		Where("provider = ?", c.Provider).Updates(map[string]any{
		"api_key":    c.APIKey,
		"base_url":   c.BaseURL,
		"extra":      c.Extra,
		"enabled":    c.Enabled,
		"updated_at": time.Now(),
	}).Error
}

// Delete removes an API config.
func (r *ApiConfigRepository) Delete(ctx context.Context, provider string) error {
	return r.db.WithContext(ctx).Where("provider = ?", provider).Delete(&model.ApiConfig{}).Error
}

// UpdateTestResult 更新测试结果。
func (r *ApiConfigRepository) UpdateTestResult(ctx context.Context, provider, result string) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&model.ApiConfig{}).
		Where("provider = ?", provider).Updates(map[string]any{
		"test_result":    result,
		"last_tested_at": &now,
	}).Error
}
