package service

import (
	"context"
	"fmt"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// Keep Emby's existing identity, release ordering, genre and multipart rules.
// Large descriptions, search aliases and playback metadata are deliberately
// excluded from the inventory pass. Only selected groups load complete rows.
var embySeriesBrowseColumns = []string{
	"media.id", "media.library_id", "media.series_id", "media.title", "media.original_name",
	"media.path", "media.relative_path", "media.created_at", "media.updated_at",
	"media.year", "media.release_date", "media.season_num", "media.episode_num",
	"media.tm_db_id", "media.bangumi_id", "media.genres", "media.languages", "media.countries", "media.nsfw",
	"media.poster_url", "media.backdrop_url", "media.part_group_key", "media.part_group_title", "media.part_index",
	"media.width", "media.height", "media.size_bytes", "media.strm_url",
}

var embyGenreColumns = []string{
	"media.library_id", "media.title", "media.original_name", "media.path", "media.relative_path",
	"media.genres", "media.languages", "media.countries", "media.nsfw",
}

// hydrateEmbySeriesPage preserves the original SQL row order before rebuilding
// selected groups: first non-empty artwork/description is order-sensitive.
// Never put projected episodes in the existing detail cache through seriesPayload.
func (e *EmbyService) hydrateEmbySeriesPage(ctx context.Context, selected []embySeriesGroup, orderedRows []model.Media, userID string, splitParts bool) ([]embySeriesGroup, error) {
	if len(selected) == 0 {
		return []embySeriesGroup{}, nil
	}
	wanted := make(map[string]bool)
	ids := make([]string, 0)
	for _, group := range selected {
		for _, row := range group.Episodes {
			if !wanted[row.ID] {
				wanted[row.ID] = true
				ids = append(ids, row.ID)
			}
		}
	}
	byID := make(map[string]model.Media, len(ids))
	for start := 0; start < len(ids); start += 400 {
		var batch []model.Media
		q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("media.id IN ?", ids[start:min(start+400, len(ids))])
		q = e.applyUserMediaVisibility(ctx, q, userID)
		if err := q.Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, row := range batch {
			byID[row.ID] = row
		}
	}
	rows, parts := []model.Media{}, []model.Media{}
	for _, sample := range orderedRows {
		if !wanted[sample.ID] {
			continue
		}
		row, ok := byID[sample.ID]
		if !ok {
			return nil, fmt.Errorf("Emby selected media %q changed or became inaccessible; retry", sample.ID)
		}
		// A placement/grouping change during the two reads must not silently
		// return a partial or different series under an old public ID.
		if row.LibraryID != sample.LibraryID || row.PartGroupKey != sample.PartGroupKey || e.seriesIDForMedia(&row) != e.seriesIDForMedia(&sample) {
			return nil, fmt.Errorf("Emby selected media %q changed groups; retry", sample.ID)
		}
		if splitParts && row.PartGroupKey != "" {
			parts = append(parts, row)
		} else {
			rows = append(rows, row)
		}
	}
	groups, err := e.seriesGroupsFromMedia(ctx, rows)
	if err != nil {
		return nil, err
	}
	groups = append(groups, e.multipartSeriesGroupsFromMedia(parts)...)
	byGroup := make(map[string]embySeriesGroup, len(groups))
	for _, group := range groups {
		byGroup[group.ID] = group
	}
	out := make([]embySeriesGroup, 0, len(selected))
	for _, sample := range selected {
		group, ok := byGroup[sample.ID]
		if !ok || len(group.Episodes) != len(sample.Episodes) {
			return nil, fmt.Errorf("Emby selected series %q changed membership; retry", sample.ID)
		}
		out = append(out, group)
	}
	return out, nil
}
