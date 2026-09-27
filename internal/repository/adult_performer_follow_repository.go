package repository

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type AdultPerformerFollowRepository struct{ db *gorm.DB }

func (r *AdultPerformerFollowRepository) ListByUser(ctx context.Context, userID string) ([]model.AdultPerformerFollow, error) {
	var rows []model.AdultPerformerFollow
	err := r.db.WithContext(ctx).
		Where("user_id = ?", strings.TrimSpace(userID)).
		Order("updated_at DESC, created_at DESC").
		Find(&rows).Error
	return rows, err
}

func (r *AdultPerformerFollowRepository) Upsert(ctx context.Context, follow *model.AdultPerformerFollow) error {
	if follow == nil {
		return errors.New("adult performer follow is nil")
	}
	var existing model.AdultPerformerFollow
	err := r.db.WithContext(ctx).Unscoped().
		Where("user_id = ? AND source = ? AND source_id = ?", follow.UserID, follow.Source, follow.SourceID).
		First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.WithContext(ctx).Create(follow).Error
	}
	if err != nil {
		return err
	}
	follow.ID = existing.ID
	return r.db.WithContext(ctx).Unscoped().Model(&existing).Updates(map[string]any{
		"name":        follow.Name,
		"name_key":    follow.NameKey,
		"image_url":   follow.ImageURL,
		"profile_url": follow.ProfileURL,
		"deleted_at":  nil,
	}).Error
}

func (r *AdultPerformerFollowRepository) DeleteOwned(ctx context.Context, userID, id string) (bool, error) {
	result := r.db.WithContext(ctx).
		Where("id = ? AND user_id = ?", strings.TrimSpace(id), strings.TrimSpace(userID)).
		Delete(&model.AdultPerformerFollow{})
	return result.RowsAffected > 0, result.Error
}
