package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SettingRepository persists key/value preferences.
type SettingRepository struct{ db *gorm.DB }

// Get returns the value or empty string when absent.
func (r *SettingRepository) Get(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.WithContext(ctx).
		Model(&model.Setting{}).
		Select("value").
		Where("key = ?", key).
		Scan(&value).Error
	return value, err
}

// Set upserts a setting value.
func (r *SettingRepository) Set(ctx context.Context, key, value string) error {
	s := model.Setting{Key: key, Value: value, UpdatedAt: time.Now()}
	return r.db.WithContext(ctx).Save(&s).Error
}

// Delete removes a setting key.
func (r *SettingRepository) Delete(ctx context.Context, key string) error {
	return r.db.WithContext(ctx).Where("key = ?", key).Delete(&model.Setting{}).Error
}

// All returns every key/value pair (used by the admin UI).
func (r *SettingRepository) All(ctx context.Context) ([]model.Setting, error) {
	var rows []model.Setting
	err := r.db.WithContext(ctx).Find(&rows).Error
	return rows, err
}
