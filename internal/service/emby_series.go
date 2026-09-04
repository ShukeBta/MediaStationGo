package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type embySeriesGroup struct {
	ID               string
	LibraryID        string
	Name             string
	PosterURL        string
	BackdropURL      string
	Overview         string
	Rating           float32
	Year             int
	ReleaseDate      string
	TMDbID           int
	BangumiID        int
	Genres           []string
	CreatedAt        time.Time
	ArtworkUpdatedAt time.Time
	Episodes         []model.Media
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
	groups, err := e.seriesGroupsFromMedia(ctx, rows)
	if err != nil {
		return embySeriesGroup{}, false, err
	}
	for _, group := range groups {
		if group.ID == id {
			e.rememberSeriesGroup(group)
			return group, true, nil
		}
	}
	if strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if group, ok, err := e.findMultipartSeriesGroup(ctx, id, userID); err != nil || ok {
			return group, ok, err
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
	groups, err := e.seriesGroupsFromMedia(ctx, rows)
	if err != nil {
		return embySeasonGroup{}, false, err
	}
	for _, series := range groups {
		for _, season := range e.seasonsForSeries(series) {
			if season.ID == id {
				e.rememberSeriesGroup(series)
				return season, true, nil
			}
		}
	}
	if season, ok, err := e.findMultipartSeasonGroup(ctx, id, userID); err != nil || ok {
		return season, ok, err
	}
	return embySeasonGroup{}, false, nil
}

func (e *EmbyService) seriesGroupsFromMedia(ctx context.Context, rows []model.Media) ([]embySeriesGroup, error) {
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
				Genres:      e.embyGenresForMedia(&row, ""),
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
		group.Genres = uniqueFoldedStrings(append(group.Genres, e.embyGenresForMedia(&row, "")...))
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
	if err := e.applyPersistedSeriesMetadata(ctx, groups); err != nil {
		return nil, err
	}
	return groups, nil
}

func (e *EmbyService) applyPersistedSeriesMetadata(ctx context.Context, groups []embySeriesGroup) error {
	if e == nil || e.repo == nil || e.repo.DB == nil || len(groups) == 0 {
		return nil
	}
	libraryIDs := make([]string, 0)
	seriesIDs := make([]string, 0)
	tmdbIDs := make([]int, 0)
	libraryIDSet := make(map[string]struct{})
	seriesIDSet := make(map[string]struct{})
	tmdbIDSet := make(map[int]struct{})
	for _, group := range groups {
		appendUniqueString(&libraryIDs, libraryIDSet, strings.TrimSpace(group.LibraryID))
		if id := strings.TrimSpace(group.ID); id != "" && !strings.HasPrefix(id, embyVirtualSeriesPrefix) {
			appendUniqueString(&seriesIDs, seriesIDSet, id)
		}
		appendUniqueInt(&tmdbIDs, tmdbIDSet, group.TMDbID)
	}

	q := e.repo.DB.WithContext(ctx).Model(&model.Series{}).Where("1 = 0")
	if len(seriesIDs) > 0 {
		q = q.Or("id IN ?", seriesIDs)
	}
	if len(libraryIDs) > 0 && len(tmdbIDs) > 0 {
		q = q.Or("library_id IN ? AND tm_db_id IN ?", libraryIDs, tmdbIDs)
	}
	var rows []model.Series
	if err := q.Order("updated_at ASC").Find(&rows).Error; err != nil {
		return err
	}
	byID := make(map[string]model.Series, len(rows))
	byTMDb := make(map[string]model.Series, len(rows))
	for _, series := range rows {
		byID[series.ID] = series
		if series.TMDbID > 0 {
			byTMDb[seriesMetadataLookupKey(series.LibraryID, strconv.Itoa(series.TMDbID))] = series
		}
	}
	for i := range groups {
		series, ok := byID[groups[i].ID]
		if !ok && groups[i].TMDbID > 0 {
			series, ok = byTMDb[seriesMetadataLookupKey(groups[i].LibraryID, strconv.Itoa(groups[i].TMDbID))]
		}
		if ok {
			applyPersistedSeriesToGroup(&groups[i], series)
		}
	}
	return nil
}

func applyPersistedSeriesToGroup(group *embySeriesGroup, series model.Series) {
	if group == nil {
		return
	}
	if value := strings.TrimSpace(series.Title); value != "" {
		group.Name = value
	}
	if value := strings.TrimSpace(series.PosterURL); value != "" {
		group.PosterURL = value
	}
	// Once a canonical Series row exists, only its backdrop is allowed to own
	// Series artwork. An empty value is preferable to an unrelated episode still.
	group.BackdropURL = strings.TrimSpace(series.BackdropURL)
	if value := strings.TrimSpace(series.Overview); value != "" {
		group.Overview = value
	}
	if series.Rating > 0 {
		group.Rating = series.Rating
	}
	if series.Year > 0 {
		group.Year = series.Year
	}
	if series.TMDbID > 0 {
		group.TMDbID = series.TMDbID
	}
	if series.BangumiID > 0 {
		group.BangumiID = series.BangumiID
	}
	group.ArtworkUpdatedAt = series.UpdatedAt
}

func seriesMetadataLookupKey(libraryID, value string) string {
	return strings.TrimSpace(libraryID) + "\x00" + strings.ToLower(strings.TrimSpace(value))
}

func appendUniqueString(values *[]string, seen map[string]struct{}, value string) {
	if value == "" {
		return
	}
	if _, ok := seen[value]; ok {
		return
	}
	seen[value] = struct{}{}
	*values = append(*values, value)
}

func appendUniqueInt(values *[]int, seen map[int]struct{}, value int) {
	if value <= 0 {
		return
	}
	if _, ok := seen[value]; ok {
		return
	}
	seen[value] = struct{}{}
	*values = append(*values, value)
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
