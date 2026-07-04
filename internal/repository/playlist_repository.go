package repository

import (
	"context"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// PlaylistRepository persists model.Playlist + PlaylistItem.
type PlaylistRepository struct{ db *gorm.DB }

// Create inserts a new playlist.
func (r *PlaylistRepository) Create(ctx context.Context, p *model.Playlist) error {
	return r.db.WithContext(ctx).Create(p).Error
}

// ListByUser returns playlists owned by a user.
func (r *PlaylistRepository) ListByUser(ctx context.Context, userID string) ([]model.Playlist, error) {
	var rows []model.Playlist
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).
		Order("created_at desc").Find(&rows).Error
	return rows, err
}
