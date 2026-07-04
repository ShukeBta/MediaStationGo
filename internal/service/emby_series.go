package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type embySeriesGroup struct {
	ID          string
	LibraryID   string
	Name        string
	PosterURL   string
	BackdropURL string
	Overview    string
	Rating      float32
	Year        int
	ReleaseDate string
	TMDbID      int
	BangumiID   int
	CreatedAt   time.Time
	Episodes    []model.Media
}

type embySeasonGroup struct {
	ID        string
	SeriesID  string
	LibraryID string
	Name      string
	SeasonNum int
	Series    embySeriesGroup
	Episodes  []model.Media
}

func (e *EmbyService) findSeriesGroup(ctx context.Context, id, userID string) (embySeriesGroup, bool, error) {
	if strings.TrimSpace(id) == "" {
		return embySeriesGroup{}, false, nil
	}
	if strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if group, ok := e.cachedSeriesGroup(id); ok {
			return group, true, nil
		}
	}
	var rows []model.Media
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("season_num > 0 OR episode_num > 0")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if !strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		q = q.Where("series_id = ?", id)
	}
	if err := q.Order("media.season_num asc, media.episode_num asc, media.created_at asc").Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return embySeriesGroup{}, false, err
	}
	for _, group := range e.seriesGroupsFromMedia(rows) {
		if group.ID == id {
			e.rememberSeriesGroup(group)
			return group, true, nil
		}
	}
	if !strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if series, err := e.repo.Series.FindByID(ctx, id); err != nil {
			return embySeriesGroup{}, false, err
		} else if series != nil {
			return embySeriesGroup{
				ID:          series.ID,
				LibraryID:   series.LibraryID,
				Name:        series.Title,
				PosterURL:   series.PosterURL,
				BackdropURL: series.BackdropURL,
				Overview:    series.Overview,
				Rating:      series.Rating,
				Year:        series.Year,
				TMDbID:      series.TMDbID,
				BangumiID:   series.BangumiID,
				CreatedAt:   series.CreatedAt,
			}, true, nil
		}
	}
	return embySeriesGroup{}, false, nil
}

func (e *EmbyService) findSeasonGroup(ctx context.Context, id, userID string) (embySeasonGroup, bool, error) {
	if strings.TrimSpace(id) == "" || !strings.HasPrefix(id, embyVirtualSeasonPrefix) {
		return embySeasonGroup{}, false, nil
	}
	if season, ok := e.cachedSeasonGroup(id); ok {
		return season, true, nil
	}
	var rows []model.Media
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("season_num > 0 OR episode_num > 0")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if err := q.
		Order("media.season_num asc, media.episode_num asc, media.created_at asc").
		Limit(embySeriesGroupingLimit).
		Find(&rows).Error; err != nil {
		return embySeasonGroup{}, false, err
	}
	for _, series := range e.seriesGroupsFromMedia(rows) {
		for _, season := range e.seasonsForSeries(series) {
			if season.ID == id {
				e.rememberSeriesGroup(series)
				return season, true, nil
			}
		}
	}
	return embySeasonGroup{}, false, nil
}

func (e *EmbyService) seriesGroupsFromMedia(rows []model.Media) []embySeriesGroup {
	byID := map[string]*embySeriesGroup{}
	order := []string{}
	for _, row := range rows {
		row := row
		seriesID := e.seriesIDForMedia(&row)
		group, ok := byID[seriesID]
		if !ok {
			group = &embySeriesGroup{
				ID:          seriesID,
				LibraryID:   row.LibraryID,
				Name:        e.seriesNameForMedia(&row),
				Year:        row.Year,
				ReleaseDate: row.ReleaseDate,
				TMDbID:      row.TMDbID,
				BangumiID:   row.BangumiID,
				CreatedAt:   row.CreatedAt,
			}
			byID[seriesID] = group
			order = append(order, seriesID)
		}
		if row.CreatedAt.After(group.CreatedAt) {
			group.CreatedAt = row.CreatedAt
		}
		if strings.TrimSpace(row.ReleaseDate) != "" && mediaReleaseSortTime(row).After(embySeriesReleaseSortTime(*group)) {
			group.ReleaseDate = row.ReleaseDate
			if row.Year > 0 {
				group.Year = row.Year
			}
		} else if group.ReleaseDate == "" && group.Year == 0 && row.Year > 0 {
			group.Year = row.Year
		}
		if group.PosterURL == "" && row.PosterURL != "" {
			group.PosterURL = row.PosterURL
		}
		if group.BackdropURL == "" && row.BackdropURL != "" {
			group.BackdropURL = row.BackdropURL
		}
		if group.Overview == "" && row.Overview != "" {
			group.Overview = row.Overview
		}
		if group.Rating == 0 && row.Rating > 0 {
			group.Rating = row.Rating
		}
		if group.Year == 0 && row.Year > 0 {
			group.Year = row.Year
		}
		group.Episodes = append(group.Episodes, row)
	}
	groups := make([]embySeriesGroup, 0, len(order))
	for _, id := range order {
		group := *byID[id]
		sort.SliceStable(group.Episodes, func(i, j int) bool {
			if group.Episodes[i].SeasonNum != group.Episodes[j].SeasonNum {
				return group.Episodes[i].SeasonNum < group.Episodes[j].SeasonNum
			}
			if group.Episodes[i].EpisodeNum != group.Episodes[j].EpisodeNum {
				return group.Episodes[i].EpisodeNum < group.Episodes[j].EpisodeNum
			}
			return group.Episodes[i].CreatedAt.Before(group.Episodes[j].CreatedAt)
		})
		groups = append(groups, group)
	}
	return groups
}

func (e *EmbyService) seasonsForSeries(series embySeriesGroup) []embySeasonGroup {
	bySeason := map[int]*embySeasonGroup{}
	order := []int{}
	for _, episode := range series.Episodes {
		seasonNum := episode.SeasonNum
		if seasonNum < 0 {
			seasonNum = 1
		}
		season, ok := bySeason[seasonNum]
		if !ok {
			season = &embySeasonGroup{
				ID:        seasonID(series.ID, seasonNum),
				SeriesID:  series.ID,
				LibraryID: series.LibraryID,
				Name:      seasonName(seasonNum),
				SeasonNum: seasonNum,
				Series:    series,
			}
			bySeason[seasonNum] = season
			order = append(order, seasonNum)
		}
		season.Episodes = append(season.Episodes, episode)
	}
	sort.Ints(order)
	out := make([]embySeasonGroup, 0, len(order))
	for _, seasonNum := range order {
		out = append(out, *bySeason[seasonNum])
	}
	return out
}
