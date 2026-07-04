package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// HistoryRepository persists model.PlaybackHistory entries. The application
// upserts on (UserID, MediaID) so resume always reads the latest position.
type HistoryRepository struct{ db *gorm.DB }

// Upsert atomically inserts/updates the resume position.
func (r *HistoryRepository) Upsert(ctx context.Context, h *model.PlaybackHistory) error {
	var existing model.PlaybackHistory
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND media_id = ?", h.UserID, h.MediaID).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(h).Error
	}
	if err != nil {
		return err
	}
	existing.PositionMs = h.PositionMs
	existing.DurationMs = h.DurationMs
	existing.WatchedAt = h.WatchedAt
	existing.Completed = h.Completed
	return r.db.WithContext(ctx).Save(&existing).Error
}

// ListByUser returns the most recent history rows for the user.
func (r *HistoryRepository) ListByUser(ctx context.Context, userID string, limit int) ([]model.PlaybackHistory, error) {
	var rows []model.PlaybackHistory
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("watched_at desc").Limit(limit).Find(&rows).Error
	return rows, err
}
