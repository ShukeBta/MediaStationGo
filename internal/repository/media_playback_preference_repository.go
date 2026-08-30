package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type MediaPlaybackPreferenceRepository struct{ db *gorm.DB }

func (r *MediaPlaybackPreferenceRepository) FindByUserAndMedia(
	ctx context.Context,
	userID string,
	mediaID string,
) (*model.UserMediaPlaybackPreference, error) {
	var row model.UserMediaPlaybackPreference
	err := withSQLiteBusyRetry(ctx, func() error {
		row = model.UserMediaPlaybackPreference{}
		return r.db.WithContext(ctx).
			Where("user_id = ? AND media_id = ?", userID, mediaID).
			First(&row).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *MediaPlaybackPreferenceRepository) Upsert(
	ctx context.Context,
	preference *model.UserMediaPlaybackPreference,
) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "media_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"subtitle_enabled",
				"subtitle_track_key",
				"audio_track_key",
				"hidden_from_resume",
				"updated_at",
			}),
		}).Create(preference).Error
	})
}

// SetHiddenFromResume 只更新“移出继续观看”标记；已存在的音轨/字幕偏好保持不变。
func (r *MediaPlaybackPreferenceRepository) SetHiddenFromResume(
	ctx context.Context,
	userID string,
	mediaID string,
	hidden bool,
) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}, {Name: "media_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"hidden_from_resume",
				"updated_at",
			}),
		}).Create(&model.UserMediaPlaybackPreference{
			UserID:           userID,
			MediaID:          mediaID,
			SubtitleEnabled:  true,
			HiddenFromResume: hidden,
		}).Error
	})
}
