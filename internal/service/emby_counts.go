package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func (e *EmbyService) ItemCounts(ctx context.Context, userID string) (map[string]any, error) {
	base := func() *gorm.DB {
		q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("deleted_at IS NULL")
		return e.applyUserMediaVisibility(ctx, q, userID)
	}

	var itemCount int64
	if err := base().Count(&itemCount).Error; err != nil {
		return nil, err
	}

	var movieCount int64
	if err := e.filterMovieItems(ctx, base()).Count(&movieCount).Error; err != nil {
		return nil, err
	}

	var episodeCount int64
	if err := e.filterEpisodeItems(ctx, base()).Count(&episodeCount).Error; err != nil {
		return nil, err
	}

	seriesCount, err := e.countVisibleSeries(ctx, userID)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"MovieCount":   movieCount,
		"SeriesCount":  seriesCount,
		"EpisodeCount": episodeCount,
		"ItemCount":    itemCount,
	}, nil
}

func (e *EmbyService) countVisibleSeries(ctx context.Context, userID string) (int, error) {
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Select("id, library_id, series_id, title, original_name, path, season_num, episode_num").
		Where("season_num > 0 OR episode_num > 0")
	q = e.applyUserMediaVisibility(ctx, q, userID)

	seen := map[string]struct{}{}
	var rows []model.Media
	err := q.Order("media.id asc").FindInBatches(&rows, 1000, func(tx *gorm.DB, batch int) error {
		for i := range rows {
			key := strings.TrimSpace(rows[i].SeriesID)
			if key == "" {
				key = stableEmbyID(embyVirtualSeriesPrefix, rows[i].LibraryID, e.seriesNameForMedia(&rows[i]))
			}
			seen[key] = struct{}{}
		}
		return nil
	}).Error
	if err != nil {
		return 0, err
	}
	return len(seen), nil
}
