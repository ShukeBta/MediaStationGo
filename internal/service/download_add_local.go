package service

import (
	"context"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (d *DownloadService) localMediaAlreadyExists(ctx context.Context, title string) bool {
	rows, ok := d.localMediaAvailabilityRows(ctx, title)
	if !ok {
		return false
	}
	return localMediaRowsMatchDownloadTitle(title, rows)
}

func (d *DownloadService) localMediaAvailabilityRows(ctx context.Context, title string) ([]model.Media, bool) {
	if d == nil || d.repo == nil || d.repo.DB == nil {
		return nil, false
	}
	if !d.repo.DB.Migrator().HasTable(&model.Media{}) {
		return nil, false
	}
	queries := localAvailabilityTitleCandidates(title)
	if len(queries) == 0 {
		return nil, false
	}
	var rows []model.Media
	db := d.repo.DB.WithContext(ctx).Model(&model.Media{})
	for i, query := range queries {
		like := "%" + query + "%"
		clause := "title LIKE ? OR original_name LIKE ? OR path LIKE ?"
		if i == 0 {
			db = db.Where(clause, like, like, like)
		} else {
			db = db.Or(clause, like, like, like)
		}
	}
	if err := db.
		Order("season_num asc, episode_num asc, created_at desc").
		Limit(200).
		Find(&rows).Error; err != nil || len(rows) == 0 {
		return nil, false
	}
	return rows, true
}

func localMediaRowsMatchDownloadTitle(title string, rows []model.Media) bool {
	wanted := episodeRefsFromTitle(title)
	if len(wanted) == 0 {
		return true
	}
	existing := map[string]struct{}{}
	hasSeriesPack := false
	for _, row := range rows {
		rowSeason, rowEpisode := localMediaRowSeasonEpisode(row)
		if rowEpisode > 0 {
			existing[episodeKey(rowSeason, rowEpisode)] = struct{}{}
			continue
		}
		if rowEpisode <= 0 && isSeriesPackTitle(row.Title+" "+row.OriginalName+" "+row.Path) {
			hasSeriesPack = true
		}
	}
	if hasSeriesPack {
		return len(wanted) == 0
	}
	for _, ref := range wanted {
		if _, ok := existing[episodeKey(ref.Season, ref.Episode)]; !ok {
			return false
		}
	}
	return true
}

func localMediaRowSeasonEpisode(row model.Media) (int, int) {
	rowSeason := row.SeasonNum
	rowEpisode := row.EpisodeNum
	if rowSeason <= 0 || rowEpisode <= 0 {
		parsedSeason, parsedEpisode := ParseEpisode(row.Path)
		if rowSeason <= 0 {
			rowSeason = parsedSeason
		}
		if rowEpisode <= 0 {
			rowEpisode = parsedEpisode
		}
	}
	if rowSeason <= 0 {
		rowSeason = 1
	}
	return rowSeason, rowEpisode
}
