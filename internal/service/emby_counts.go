package service

import (
	"context"
	"strings"
	"time"

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

type embyLibraryViewCounts struct {
	Recursive int
	Child     int
	Unplayed  int
}

type embyMediaCounts struct {
	MovieCount   int
	SeriesCount  int
	EpisodeCount int
	ItemCount    int
}

func (e *EmbyService) libraryViewCounts(ctx context.Context, userID string, l *model.Library, viewID, collectionType string) embyLibraryViewCounts {
	if e == nil || l == nil {
		return embyLibraryViewCounts{}
	}
	counts, err := e.mediaCountsForParent(ctx, userID, viewID)
	if err != nil {
		return embyLibraryViewCounts{}
	}
	switch strings.ToLower(strings.TrimSpace(collectionType)) {
	case "tvshows":
		return embyLibraryViewCounts{Recursive: counts.EpisodeCount, Child: counts.SeriesCount, Unplayed: counts.EpisodeCount}
	case "movies":
		return embyLibraryViewCounts{Recursive: counts.MovieCount, Child: counts.MovieCount, Unplayed: counts.MovieCount}
	default:
		recursive := counts.MovieCount + counts.EpisodeCount
		child := counts.MovieCount + counts.SeriesCount
		if recursive == 0 {
			recursive = counts.ItemCount
		}
		if child == 0 {
			child = recursive
		}
		return embyLibraryViewCounts{Recursive: recursive, Child: child, Unplayed: recursive}
	}
}

func (e *EmbyService) mediaCountsForParent(ctx context.Context, userID, parentID string) (embyMediaCounts, error) {
	if e == nil || e.repo == nil || e.repo.DB == nil {
		return embyMediaCounts{}, nil
	}
	var (
		libraryIDs    []string
		kind          string
		countParentID string
	)
	parentID = strings.TrimSpace(parentID)
	if libraryID, virtualKind, ok := parseVirtualLibraryID(parentID); ok {
		libraryIDs = e.mergedLibraryIDs(ctx, libraryID)
		kind = virtualKind
		countParentID = libraryID
	} else if parentID != "" {
		if lib, err := e.repo.Library.FindByID(ctx, parentID); err != nil {
			return embyMediaCounts{}, err
		} else if lib != nil {
			libraryIDs = e.mergedLibraryIDs(ctx, parentID)
			countParentID = parentID
		} else {
			return embyMediaCounts{}, nil
		}
	}
	counts, err := e.mediaCountsForLibraryIDs(ctx, userID, countParentID, libraryIDs)
	if err != nil {
		return embyMediaCounts{}, err
	}
	return counts.forVirtualKind(kind), nil
}

func (e *EmbyService) mediaCountsForLibraryIDs(ctx context.Context, userID, cacheParentID string, libraryIDs []string) (embyMediaCounts, error) {
	cacheKey := e.embyCountsCacheKey(userID, cacheParentID)
	var cached embyCountsCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		return cached.Counts, nil
	}
	base := e.repo.DB.WithContext(ctx).Model(&model.Media{})
	if len(libraryIDs) > 0 {
		base = base.Where("library_id IN ?", libraryIDs)
	}
	base = e.applyUserMediaVisibility(ctx, base, userID)

	var agg struct {
		Total            int64
		SeriesIDEpisodes int64
		SeriesWithID     int64
	}
	if err := base.Session(&gorm.Session{}).Select(`
		COUNT(*) AS total,
		SUM(CASE WHEN series_id IS NOT NULL AND series_id <> '' THEN 1 ELSE 0 END) AS series_id_episodes,
		COUNT(DISTINCT NULLIF(series_id, '')) AS series_with_id
	`).Scan(&agg).Error; err != nil {
		return embyMediaCounts{}, err
	}

	noSeriesRows, err := e.noSeriesEpisodeCandidateRows(ctx, base)
	if err != nil {
		return embyMediaCounts{}, err
	}
	libraryTypes := e.libraryTypesForRows(ctx, noSeriesRows)
	noSeriesEpisodeRows := make([]model.Media, 0, len(noSeriesRows))
	for i := range noSeriesRows {
		if embyMediaShouldBeEpisodeWithLibraryTypes(&noSeriesRows[i], libraryTypes) {
			noSeriesEpisodeRows = append(noSeriesEpisodeRows, noSeriesRows[i])
		}
	}
	episodeCount := int(agg.SeriesIDEpisodes) + len(noSeriesEpisodeRows)
	movieCount := int(agg.Total) - episodeCount
	if movieCount < 0 {
		movieCount = 0
	}
	counts := embyMediaCounts{
		MovieCount:   movieCount,
		SeriesCount:  int(agg.SeriesWithID) + len(e.seriesGroupsFromMedia(noSeriesEpisodeRows)),
		EpisodeCount: episodeCount,
	}
	counts.ItemCount = counts.MovieCount + counts.EpisodeCount
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyCountsCacheValue{Counts: counts}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
	}
	return counts, nil
}

func (e *EmbyService) noSeriesEpisodeCandidateRows(ctx context.Context, base *gorm.DB) ([]model.Media, error) {
	q := base.Session(&gorm.Session{}).
		Select("id", "library_id", "series_id", "title", "original_name", "path", "season_num", "episode_num", "created_at", "year", "tm_db_id", "bangumi_id").
		Where("(series_id IS NULL OR series_id = '') AND (season_num > 0 OR episode_num > 0)")
	var rows []model.Media
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (c embyMediaCounts) forVirtualKind(kind string) embyMediaCounts {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "movies":
		return embyMediaCounts{MovieCount: c.MovieCount, ItemCount: c.MovieCount}
	case "shows", "tvshows", "series":
		return embyMediaCounts{SeriesCount: c.SeriesCount, EpisodeCount: c.EpisodeCount, ItemCount: c.EpisodeCount}
	default:
		return c
	}
}

func (e *EmbyService) libraryTypesForRows(ctx context.Context, rows []model.Media) map[string]string {
	if e == nil || e.repo == nil || e.repo.DB == nil || len(rows) == 0 {
		return nil
	}
	ids := make([]string, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		id := strings.TrimSpace(row.LibraryID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	var libs []model.Library
	if err := e.repo.DB.WithContext(ctx).Model(&model.Library{}).Select("id", "type").Where("id IN ?", ids).Find(&libs).Error; err != nil {
		return nil
	}
	out := make(map[string]string, len(libs))
	for _, lib := range libs {
		out[lib.ID] = lib.Type
	}
	return out
}

func embyMediaShouldBeEpisodeWithLibraryTypes(m *model.Media, libraryTypes map[string]string) bool {
	if m == nil {
		return false
	}
	if strings.TrimSpace(m.SeriesID) != "" {
		return true
	}
	if m.SeasonNum <= 0 && m.EpisodeNum <= 0 {
		return false
	}
	libraryType := libraryTypes[strings.TrimSpace(m.LibraryID)]
	if embyLibraryTypeIsEpisodic(libraryType) {
		return true
	}
	if embyMediaPathLooksEpisodic(m.Path) {
		return true
	}
	return embyLibraryTypeAllowsFilenameEpisodeSignal(libraryType) && embyMediaHasStrongEpisodeSignal(m)
}

type embyLibraryMediaShape struct {
	HasMovies   bool
	HasEpisodes bool
}

func (e *EmbyService) libraryCollectionType(ctx context.Context, l *model.Library) string {
	if l == nil {
		return "movies"
	}
	if strings.EqualFold(strings.TrimSpace(l.Type), "music") {
		return "music"
	}
	shape, err := e.libraryMediaShape(ctx, l.ID)
	if err == nil {
		switch {
		case shape.HasMovies && shape.HasEpisodes:
			return "mixed"
		case shape.HasEpisodes:
			return "tvshows"
		case shape.HasMovies:
			return "movies"
		}
	}
	switch strings.ToLower(strings.TrimSpace(l.Type)) {
	case "tv", "anime", "variety":
		return "tvshows"
	case "music":
		return "music"
	default:
		return "movies"
	}
}

func (e *EmbyService) libraryMediaShape(ctx context.Context, libraryID string) (embyLibraryMediaShape, error) {
	var shape embyLibraryMediaShape
	if e == nil || e.repo == nil || strings.TrimSpace(libraryID) == "" {
		return shape, nil
	}
	var rows []model.Media
	err := e.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("id", "library_id", "series_id", "title", "original_name", "path", "season_num", "episode_num").
		Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID)).
		Order("media.created_at desc").
		Limit(embySeriesGroupingLimit).
		Find(&rows).Error
	if err != nil {
		return shape, err
	}
	for i := range rows {
		if e.mediaShouldBeEpisode(ctx, &rows[i]) {
			shape.HasEpisodes = true
		} else {
			shape.HasMovies = true
		}
		if shape.HasMovies && shape.HasEpisodes {
			break
		}
	}
	return shape, nil
}
