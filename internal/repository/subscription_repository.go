package repository

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SubscriptionRepository persists model.Subscription records.
type SubscriptionRepository struct{ db *gorm.DB }

// Create inserts a new subscription rule.
func (r *SubscriptionRepository) Create(ctx context.Context, s *model.Subscription) error {
	return r.db.WithContext(ctx).Select("*").Omit("DeletedAt").Create(s).Error
}

// List returns active subscription rules. Archived rows live in history and are
// intentionally excluded from scheduler polling and the active management list.
func (r *SubscriptionRepository) List(ctx context.Context) ([]model.Subscription, error) {
	var rows []model.Subscription
	err := r.db.WithContext(ctx).Unscoped().
		Where("archived_at IS NULL").
		Where("deleted_at IS NULL OR enabled = ?", true).
		Order("created_at desc").
		Find(&rows).Error
	return rows, err
}

// History returns archived subscription rules.
func (r *SubscriptionRepository) History(ctx context.Context) ([]model.Subscription, error) {
	var rows []model.Subscription
	err := r.db.WithContext(ctx).Where("archived_at IS NOT NULL").Order("archived_at desc, updated_at desc").Find(&rows).Error
	return rows, err
}

// Archive moves a completed subscription out of the active list without
// deleting its rule details, so users can audit completed subscriptions later.
func (r *SubscriptionRepository) Archive(ctx context.Context, id, reason string, archivedAt time.Time) error {
	return r.db.WithContext(ctx).Model(&model.Subscription{}).
		Where("id = ? AND archived_at IS NULL", id).
		Updates(map[string]any{
			"enabled":        false,
			"archived_at":    &archivedAt,
			"archive_reason": reason,
		}).Error
}
