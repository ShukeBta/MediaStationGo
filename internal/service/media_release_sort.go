package service

import (
	"fmt"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func mediaReleaseSortTime(media model.Media) time.Time {
	if value := normalizeReleaseDate(media.ReleaseDate); value != "" {
		if t, err := time.Parse("2006-01-02", value); err == nil {
			return t
		}
	}
	if media.Year > 0 {
		if t, err := time.Parse("2006-01-02", fmt.Sprintf("%04d-12-31", media.Year)); err == nil {
			return t
		}
	}
	if !media.UpdatedAt.IsZero() {
		return media.UpdatedAt
	}
	return media.CreatedAt
}

func mediaReleaseOrderSQL(desc bool) string {
	dir := "ASC"
	if desc {
		dir = "DESC"
	}
	return fmt.Sprintf("media.release_date %s, media.year %s, media.created_at %s, media.id %s", dir, dir, dir, dir)
}

func embyPremiereDate(value string) (time.Time, bool) {
	value = normalizeReleaseDate(value)
	if value == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", value)
	return t, err == nil
}
