package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (e *EmbyService) mediaItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	cacheKey := e.embyItemsCacheKey("items", p)
	var cached embyItemsCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		return map[string]any{"Items": cached.Items, "TotalRecordCount": cached.TotalRecordCount, "StartIndex": cached.StartIndex}, nil
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{})
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if p.ParentID != "" {
		q = q.Where("library_id IN ? OR series_id = ?", e.mergedLibraryIDs(ctx, p.ParentID), p.ParentID)
	}
	if p.SearchTerm != "" {
		q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
	}
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": int64(0), "StartIndex": p.StartIndex}, nil
		}
		q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
	}
	resumeFilter := containsEmbyFilter(p.Filters, "IsResumable")
	if resumeFilter {
		if strings.TrimSpace(p.UserID) == "" {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": int64(0), "StartIndex": p.StartIndex}, nil
		}
		q = q.Joins(`JOIN (
			SELECT media_id, MAX(watched_at) AS watched_at
			FROM playback_histories
			WHERE user_id = ? AND completed = ? AND position_ms > 0
			GROUP BY media_id
		) AS resume ON resume.media_id = media.id`, p.UserID, false)
	}
	filterBySeasonNumbers := true
	parentKnownNonEpisodic := false
	if p.ParentID != "" {
		if episodic, err := e.libraryIsEpisodic(ctx, p.ParentID); err == nil && !episodic {
			filterBySeasonNumbers = false
			parentKnownNonEpisodic = true
		}
	}
	if parentKnownNonEpisodic && containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Movie") {
		return emptyItemsEnvelope(p.StartIndex), nil
	}
	if filterBySeasonNumbers && containsItemType(p.IncludeItemTypes, "Movie") && !containsItemType(p.IncludeItemTypes, "Episode") {
		q = e.filterMovieItems(ctx, q)
	}
	if parentKnownNonEpisodic && containsItemType(p.IncludeItemTypes, "Movie") && !containsItemType(p.IncludeItemTypes, "Episode") {
		q = filterLikelyEpisodicPathsFromMovieQuery(q)
	}
	if filterBySeasonNumbers && containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Movie") {
		q = e.filterEpisodeItems(ctx, q)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	desc := !strings.EqualFold(firstCSVValue(p.SortOrder), "Ascending")
	order := mediaReleaseOrderSQL(true)
	orderIncludesDirection := true
	switch primarySupportedEmbySort(p.SortBy, resumeFilter) {
	case "sortname", "name":
		order = "media.title"
		orderIncludesDirection = false
	case "premieredate", "productionyear":
		order = mediaReleaseOrderSQL(desc)
	case "datecreated":
		order = "media.created_at"
		orderIncludesDirection = false
	case "dateplayed":
		order = "resume.watched_at"
		orderIncludesDirection = false
	case "communityrating":
		order = "media.rating"
		orderIncludesDirection = false
	}
	if !orderIncludesDirection && strings.EqualFold(firstCSVValue(p.SortOrder), "Descending") {
		if !strings.HasSuffix(order, " desc") {
			order = order + " desc"
		}
	}

	fetchLimit := p.Limit
	fetchOffset := p.StartIndex
	if fetchLimit > 0 && e.shouldCollapseMediaVersions(ctx, p) {
		// Duplicates across merged local/cloud libraries collapse into one Emby
		// item with multiple MediaSources. Fetch a wider window so duplicates do
		// not consume the whole requested page.
		fetchOffset = 0
		fetchLimit = p.StartIndex + maxInt(p.Limit*4, p.Limit)
	}
	var rows []model.Media
	if err := q.Order(order).Offset(fetchOffset).Limit(fetchLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	if e.shouldCollapseMediaVersions(ctx, p) {
		rows = e.collapseMediaVersionRows(ctx, rows)
		rows = pageSlice(rows, p.StartIndex, p.Limit)
	}
	items, err := e.payloadsForMedia(ctx, rows, p.UserID)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyItemsCacheValue{Items: items, TotalRecordCount: total, StartIndex: p.StartIndex}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
	}
	return out, nil
}

func (e *EmbyService) episodeItems(ctx context.Context, rows []model.Media, p ItemsParams) (map[string]any, error) {
	rows = e.filterMediaRowsForUser(ctx, rows, p.UserID)
	if p.SearchTerm != "" {
		filtered := rows[:0]
		needle := strings.ToLower(p.SearchTerm)
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.Title), needle) || strings.Contains(strings.ToLower(row.OriginalName), needle) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SeasonNum != rows[j].SeasonNum {
			return rows[i].SeasonNum < rows[j].SeasonNum
		}
		if rows[i].EpisodeNum != rows[j].EpisodeNum {
			return rows[i].EpisodeNum < rows[j].EpisodeNum
		}
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
	total := len(rows)
	items, err := e.payloadsForMedia(ctx, pageSlice(rows, p.StartIndex, p.Limit), p.UserID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) payloadsForMedia(ctx context.Context, rows []model.Media, userID string) ([]map[string]any, error) {
	rows = e.collapseMediaVersionRows(ctx, rows)
	userFavs := map[string]bool{}
	userPos := map[string]int64{}
	if userID != "" && len(rows) > 0 {
		mediaIDs := make([]string, 0, len(rows))
		for _, row := range rows {
			if strings.TrimSpace(row.ID) != "" {
				mediaIDs = append(mediaIDs, row.ID)
			}
		}
		if len(mediaIDs) == 0 {
			mediaIDs = []string{"__none__"}
		}
		var favs []model.Favorite
		favQuery := e.repo.DB.WithContext(ctx).Where("user_id = ?", userID).Where("media_id IN ?", mediaIDs)
		_ = favQuery.Find(&favs).Error
		for _, f := range favs {
			userFavs[f.MediaID] = true
		}
		var hist []model.PlaybackHistory
		histQuery := e.repo.DB.WithContext(ctx).Where("user_id = ?", userID).Where("media_id IN ?", mediaIDs)
		_ = histQuery.Find(&hist).Error
		for _, h := range hist {
			userPos[h.MediaID] = h.PositionMs
		}
	}

	items := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		items = append(items, e.itemPayload(ctx, &m, userFavs[m.ID], userPos[m.ID]))
	}
	return items, nil
}

func (e *EmbyService) shouldCollapseMediaVersions(ctx context.Context, p ItemsParams) bool {
	if containsItemType(p.IncludeItemTypes, "Series") || containsItemType(p.IncludeItemTypes, "Season") {
		return false
	}
	if containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Movie") {
		return true
	}
	if p.ParentID == "" {
		return true
	}
	episodic, err := e.libraryIsEpisodic(ctx, p.ParentID)
	return err == nil && !episodic
}

func (e *EmbyService) collapseMediaVersionRows(ctx context.Context, rows []model.Media) []model.Media {
	if len(rows) < 2 {
		return rows
	}
	out := make([]model.Media, 0, len(rows))
	indexByKey := make(map[string]int, len(rows))
	for _, row := range rows {
		key := e.mediaVersionKey(ctx, &row)
		if key == "" {
			out = append(out, row)
			continue
		}
		if idx, ok := indexByKey[key]; ok {
			if preferMediaVersion(row, out[idx]) {
				out[idx] = row
			}
			continue
		}
		indexByKey[key] = len(out)
		out = append(out, row)
	}
	return out
}

func (e *EmbyService) seriesItemsForLibrary(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("season_num > 0 OR episode_num > 0")
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if libraryID != "" {
		q = q.Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	}
	if p.SearchTerm != "" {
		q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
	}
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
		}
		q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
	}
	var rows []model.Media
	if err := q.Order(mediaReleaseOrderSQL(true)).Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	groups := e.seriesGroupsFromMedia(rows)
	sortSeriesGroups(groups, p)
	total := len(groups)
	items := make([]map[string]any, 0, minInt(p.Limit, len(groups)))
	for _, group := range pageSlice(groups, p.StartIndex, p.Limit) {
		items = append(items, e.seriesPayload(group))
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}
