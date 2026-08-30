package repository

import (
	"context"
	"errors"
	"time"

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

// ClearHiddenFromResume 在该条目产生新的播放进度时撤销“移出继续观看”，
// 对应标准 Emby 行为：清掉的条目再次观看会回到继续观看列表。
// 只更新已存在的隐藏行，不为没有隐藏过的条目制造偏好行。
func (r *MediaPlaybackPreferenceRepository) ClearHiddenFromResume(
	ctx context.Context,
	userID string,
	mediaID string,
) error {
	return withSQLiteBusyRetry(ctx, func() error {
		return r.db.WithContext(ctx).Exec(
			"UPDATE user_media_playback_preferences SET hidden_from_resume = false, updated_at = ? "+
				"WHERE user_id = ? AND media_id = ? AND hidden_from_resume = true AND deleted_at IS NULL",
			time.Now(), userID, mediaID,
		).Error
	})
}
