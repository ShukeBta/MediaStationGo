package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// AccessLogRepository persists model.AccessLog records.
type AccessLogRepository struct{ db *gorm.DB }

// Create inserts one structured audit-trail entry.
func (r *AccessLogRepository) Create(ctx context.Context, l *model.AccessLog) error {
	return r.db.WithContext(ctx).Create(l).Error
}

// Recent returns the latest access-log entries (admin Activity panel).
func (r *AccessLogRepository) Recent(ctx context.Context, limit int) ([]model.AccessLog, error) {
	var rows []model.AccessLog
	err := r.db.WithContext(ctx).Order("created_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}
