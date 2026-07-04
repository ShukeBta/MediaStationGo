package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// FavoriteRepository persists model.Favorite records.
type FavoriteRepository struct{ db *gorm.DB }

// Toggle flips the favourite flag for (user, media). Returns the new state.
func (r *FavoriteRepository) Toggle(ctx context.Context, userID, mediaID string) (bool, error) {
	var f model.Favorite
	err := r.db.WithContext(ctx).Where("user_id = ? AND media_id = ?", userID, mediaID).First(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		fav := model.Favorite{UserID: userID, MediaID: mediaID}
		return true, r.db.WithContext(ctx).Create(&fav).Error
	}
	if err != nil {
		return false, err
	}
	return false, r.db.WithContext(ctx).Delete(&f).Error
}

// ListByUser returns all favourite media IDs for a user.
func (r *FavoriteRepository) ListByUser(ctx context.Context, userID string) ([]model.Favorite, error) {
	var rows []model.Favorite
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&rows).Error
	return rows, err
}
