package service

import (
	"context"
	"errors"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"gorm.io/gorm"
	"strings"
)

const autoMarkPreviousEpisodesSetting = "playback.auto_mark_previous_episodes"

// savePlaybackProgress preserves the physical-media history schema used by
// SQLite and PostgreSQL. Inferred completion does not represent a playback event.
func savePlaybackProgress(ctx context.Context, repo *repository.Container, history *model.PlaybackHistory, visibility MediaVisibility) error {
	autoMark := false
	if history.Completed && repo.Setting != nil && repo.DB.Migrator().HasTable(&model.Setting{}) {
		value, err := repo.Setting.Get(ctx, autoMarkPreviousEpisodesSetting)
		if err != nil {
			return err
		}
		autoMark = parseBoolSetting(value, false)
	}
	if !autoMark {
		return repo.History.Upsert(ctx, history)
	}
	return repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := repository.New(tx).History.Upsert(ctx, history); err != nil {
			return err
		}
		return markPreviousEpisodes(ctx, tx, history, visibility)
	})
}

func markPreviousEpisodes(ctx context.Context, db *gorm.DB, history *model.PlaybackHistory, visibility MediaVisibility) error {
	var current model.Media
	if err := db.WithContext(ctx).First(&current, "id = ?", history.MediaID).Error; err != nil {
		return err
	}
	if current.EpisodeNum <= 1 || current.SeasonNum < 0 || !visibility.Allows(&current) {
		return nil
	}
	query := db.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND season_num = ? AND episode_num > 0 AND episode_num < ?", current.LibraryID, current.SeasonNum, current.EpisodeNum).
		Where("TRIM(path) <> '' OR TRIM(strm_url) <> ''")
	// Never infer a relationship from a display title alone.
	switch {
	case strings.TrimSpace(current.SeriesID) != "":
		query = query.Where("series_id = ?", current.SeriesID)
	case strings.TrimSpace(current.SeriesKey) != "":
		query = query.Where("series_key = ?", current.SeriesKey)
	case current.TMDbID > 0:
		query = query.Where("tm_db_id = ?", current.TMDbID)
	default:
		return nil
	}
	var rows []model.Media
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if !visibility.Allows(&row) {
			continue
		}
		var existing model.PlaybackHistory
		err := db.WithContext(ctx).Where("user_id = ? AND media_id = ?", history.UserID, row.ID).First(&existing).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && existing.Completed {
			continue
		}
		duration := max(int64(0), int64(row.DurationSec)*1000)
		if existing.DurationMs > 0 {
			duration = existing.DurationMs
		}
		if err == nil {
			// A concurrent real completion must keep its original timestamp.
			if err := db.WithContext(ctx).Model(&model.PlaybackHistory{}).Where("id = ? AND completed = ?", existing.ID, false).
				Updates(map[string]any{"position_ms": duration, "duration_ms": duration, "completed": true, "watched_at": history.WatchedAt}).Error; err != nil {
				return err
			}
		} else {
			inferred := model.PlaybackHistory{UserID: history.UserID, MediaID: row.ID, PositionMs: duration, DurationMs: duration, Completed: true, WatchedAt: history.WatchedAt}
			if err := db.WithContext(ctx).Create(&inferred).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
