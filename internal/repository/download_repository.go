package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// DownloadRepository persists model.DownloadTask records.
type DownloadRepository struct{ db *gorm.DB }

// Create inserts a new download task.
func (r *DownloadRepository) Create(ctx context.Context, t *model.DownloadTask) error {
	return r.db.WithContext(ctx).Create(t).Error
}

// List returns all download tasks (admin view).
func (r *DownloadRepository) List(ctx context.Context) ([]model.DownloadTask, error) {
	var rows []model.DownloadTask
	err := r.db.WithContext(ctx).Order("created_at desc").Find(&rows).Error
	return rows, err
}
