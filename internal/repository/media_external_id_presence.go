package repository

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type externalIDPresenceRow struct {
	TMDbID   int    `gorm:"column:tm_db_id"`
	DoubanID string `gorm:"column:douban_id"`
	SeriesID string `gorm:"column:series_id"`
}

// ExternalIDReference identifies a TMDB media type and ID. TMDB uses separate
// numeric namespaces for movies and TV series, so the type is part of identity.
type ExternalIDReference struct {
	ID        int
	MediaType string
}

// FindExternalIDPresence reports only the requested external IDs that exist in
// media rows visible under the supplied query filter.
func (r *MediaRepository) FindExternalIDPresence(
	ctx context.Context,
	tmdbRefs []ExternalIDReference,
	doubanIDs []string,
	filter MediaQueryFilter,
) (map[ExternalIDReference]bool, map[string]bool, error) {
	tmdbWanted := make(map[ExternalIDReference]struct{}, len(tmdbRefs))
	for _, ref := range tmdbRefs {
		if ref.ID > 0 {
			tmdbWanted[ref] = struct{}{}
		}
	}
	doubanWanted := make(map[string]struct{}, len(doubanIDs))
	for _, id := range doubanIDs {
		if id = strings.TrimSpace(id); id != "" {
			doubanWanted[id] = struct{}{}
		}
	}
	tmdbPresent := make(map[ExternalIDReference]bool, len(tmdbWanted))
	doubanPresent := make(map[string]bool, len(doubanWanted))
	if len(tmdbWanted) == 0 && len(doubanWanted) == 0 {
		return tmdbPresent, doubanPresent, nil
	}

	tmdbValues := make([]int, 0, len(tmdbWanted))
	seenTMDbValues := make(map[int]struct{}, len(tmdbWanted))
	for ref := range tmdbWanted {
		if _, seen := seenTMDbValues[ref.ID]; seen {
			continue
		}
		seenTMDbValues[ref.ID] = struct{}{}
		tmdbValues = append(tmdbValues, ref.ID)
	}
	doubanValues := make([]string, 0, len(doubanWanted))
	for id := range doubanWanted {
		doubanValues = append(doubanValues, id)
	}

	q := r.db.WithContext(ctx).Model(&model.Media{}).Select("tm_db_id", "douban_id", "series_id")
	q = applyMediaQueryFilter(q, filter)
	switch {
	case len(tmdbValues) > 0 && len(doubanValues) > 0:
		q = q.Where("(tm_db_id IN ? OR douban_id IN ?)", tmdbValues, doubanValues)
	case len(tmdbValues) > 0:
		q = q.Where("tm_db_id IN ?", tmdbValues)
	default:
		q = q.Where("douban_id IN ?", doubanValues)
	}

	var rows []externalIDPresenceRow
	if err := q.Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		mediaType := "movie"
		if strings.TrimSpace(row.SeriesID) != "" {
			mediaType = "tv"
		}
		for ref := range tmdbWanted {
			if ref.ID == row.TMDbID && (ref.MediaType == "" || ref.MediaType == mediaType) {
				tmdbPresent[ref] = true
			}
		}
		if _, ok := doubanWanted[row.DoubanID]; ok {
			doubanPresent[row.DoubanID] = true
		}
	}
	return tmdbPresent, doubanPresent, nil
}
