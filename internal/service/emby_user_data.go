package service

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SetFavorite 把 mediaID 标为 userID 的收藏。
func (e *EmbyService) SetFavorite(ctx context.Context, userID, mediaID string, favorite bool) error {
	if favorite {
		var f model.Favorite
		err := e.repo.DB.WithContext(ctx).
			Where("user_id = ? AND media_id = ?", userID, mediaID).First(&f).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return e.repo.DB.WithContext(ctx).Create(&model.Favorite{
				UserID: userID, MediaID: mediaID,
			}).Error
		}
		return err
	}
	return e.repo.DB.WithContext(ctx).
		Where("user_id = ? AND media_id = ?", userID, mediaID).
		Delete(&model.Favorite{}).Error
}

// MarkPlayed 把 mediaID 标为已看（写一个 100% 进度的 history 行）。
func (e *EmbyService) MarkPlayed(ctx context.Context, userID, mediaID string, played bool) error {
	if !played {
		return e.repo.DB.WithContext(ctx).
			Where("user_id = ? AND media_id = ?", userID, mediaID).
			Delete(&model.PlaybackHistory{}).Error
	}
	m, err := e.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return errors.New("media not found")
	}
	dur := int64(m.DurationSec) * 1000
	if dur <= 0 {
		dur = 1
	}
	return e.repo.History.Upsert(ctx, &model.PlaybackHistory{
		UserID:     userID,
		MediaID:    mediaID,
		PositionMs: dur,
		DurationMs: dur,
		WatchedAt:  time.Now(),
		Completed:  true,
	})
}

// RecordProgress 记录播放进度（来自 Emby 客户端的 /Sessions/Playing/Progress）。
func (e *EmbyService) RecordProgress(ctx context.Context, userID, mediaID string, positionTicks, runtimeTicks int64) error {
	if e.playback != nil {
		if err := e.playback.ValidateProgressWrite(ctx, userID, mediaID); err != nil {
			return err
		}
	}
	pos := positionTicks / 10_000
	dur := runtimeTicks / 10_000
	if dur <= 0 {
		// runtimeTicks 缺失时回退到 media.DurationSec
		if m, _ := e.repo.Media.FindByID(ctx, mediaID); m != nil {
			dur = int64(m.DurationSec) * 1000
		}
	}
	completed := dur > 0 && pos >= dur*9/10
	if err := e.repo.History.Upsert(ctx, &model.PlaybackHistory{
		UserID:     userID,
		MediaID:    mediaID,
		PositionMs: pos,
		DurationMs: dur,
		WatchedAt:  time.Now(),
		Completed:  completed,
	}); err != nil {
		return err
	}
	// 标准行为：被移出继续观看的条目再次观看时自动恢复。
	return e.repo.MediaPlaybackPreference.ClearHiddenFromResume(ctx, userID, mediaID)
}

// SetHiddenFromResume 按 Emby Hide 查询参数更新该用户的“移出继续观看”状态。
// Hide=false 必须撤销隐藏，不能和 Hide=true 一样写成隐藏。
func (e *EmbyService) SetHiddenFromResume(ctx context.Context, userID, mediaID string, hidden bool) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(mediaID) == "" {
		return errors.New("missing user or media")
	}
	if !hidden {
		return e.repo.MediaPlaybackPreference.ClearHiddenFromResume(ctx, userID, mediaID)
	}
	return e.repo.MediaPlaybackPreference.SetHiddenFromResume(ctx, userID, mediaID, true)
}

// MediaUserData returns the small Emby user-state payload used by action
// routes. It deliberately avoids constructing a complete Item (artwork,
// people, media sources and library hierarchy are irrelevant to the reply).
func (e *EmbyService) MediaUserData(ctx context.Context, userID, mediaID string) (map[string]any, bool, error) {
	var media model.Media
	mediaQuery := e.repo.DB.WithContext(ctx).Model(&model.Media{})
	mediaQuery = e.applyUserMediaVisibility(ctx, mediaQuery, userID)
	if err := mediaQuery.
		Select("id", "duration_sec").
		Where("id = ?", mediaID).
		First(&media).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}

	favorite := false
	positionMs := int64(0)
	watchedAt := time.Time{}
	if strings.TrimSpace(userID) != "" {
		var favoriteCount int64
		if err := e.repo.DB.WithContext(ctx).Model(&model.Favorite{}).
			Where("user_id = ? AND media_id = ?", userID, mediaID).
			Count(&favoriteCount).Error; err != nil {
			return nil, false, err
		}
		favorite = favoriteCount > 0

		var history model.PlaybackHistory
		err := e.repo.DB.WithContext(ctx).
			Select("position_ms", "watched_at").
			Where("user_id = ? AND media_id = ?", userID, mediaID).
			Order("watched_at DESC, updated_at DESC, id DESC").
			First(&history).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
		if err == nil {
			positionMs = history.PositionMs
			watchedAt = history.WatchedAt
		}
	}

	return embyUserDataPayload(favorite, positionMs, int64(media.DurationSec)*1000, watchedAt), true, nil
}

func embyUserDataPayload(favorite bool, positionMs, durationMs int64, watchedAt time.Time) map[string]any {
	played := positionMs > 0 && durationMs > 0 && positionMs >= durationMs*9/10
	percentage := 0.0
	if durationMs > 0 {
		percentage = float64(positionMs) / float64(durationMs) * 100
	}
	userData := map[string]any{
		"PlaybackPositionTicks": positionMs * 10_000,
		"PlayCount":             0,
		"IsFavorite":            favorite,
		"Played":                played,
		"PlayedPercentage":      percentage,
	}
	if !watchedAt.IsZero() {
		userData["LastPlayedDate"] = watchedAt.UTC().Format(time.RFC3339Nano)
	}
	return userData
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func intToStr(v int) string {
	if v == 0 {
		return ""
	}
	return strconv.Itoa(v)
}
