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
	rows = e.filterEpisodeRows(ctx, rows)
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

type embyTopLevelEntry struct {
	Payload   map[string]any
	Name      string
	CreatedAt time.Time
	Year      int
	Rating    float32
}

func (e *EmbyService) itemsForVirtualLibrary(ctx context.Context, libraryID, kind string, p ItemsParams) (map[string]any, error) {
	p.ParentID = libraryID
	switch kind {
	case "movies":
		if len(p.IncludeItemTypes) > 0 && !containsItemType(p.IncludeItemTypes, "Movie") && !containsItemType(p.IncludeItemTypes, "Video") {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
		return e.movieItemsForLibrary(ctx, libraryID, p)
	case "shows":
		if len(p.IncludeItemTypes) > 0 &&
			!containsItemType(p.IncludeItemTypes, "Series") &&
			!containsItemType(p.IncludeItemTypes, "Season") &&
			!containsItemType(p.IncludeItemTypes, "Episode") &&
			!containsItemType(p.IncludeItemTypes, "Folder") {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
		if containsItemType(p.IncludeItemTypes, "Episode") && !containsItemType(p.IncludeItemTypes, "Series") {
			return e.episodeItemsForLibrary(ctx, libraryID, p)
		}
		return e.seriesItemsForLibrary(ctx, libraryID, p)
	default:
		return emptyItemsEnvelope(p.StartIndex), nil
	}
}

func (e *EmbyService) libraryTopLevelItems(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	includeMovies := len(p.IncludeItemTypes) == 0 || containsItemType(p.IncludeItemTypes, "Movie")
	includeSeries := len(p.IncludeItemTypes) == 0 || containsItemType(p.IncludeItemTypes, "Series")
	if !includeMovies && !includeSeries {
		return emptyItemsEnvelope(p.StartIndex), nil
	}

	rowLimit := p.StartIndex + maxInt(p.Limit*40, 1000)
	if rowLimit < p.Limit {
		rowLimit = p.Limit
	}
	if rowLimit > embySeriesGroupingLimit {
		rowLimit = embySeriesGroupingLimit
	}

	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if p.SearchTerm != "" {
		q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
	}
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
		q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
	}

	var rows []model.Media
	if err := q.Order(mediaReleaseOrderSQL(true)).Limit(rowLimit).Find(&rows).Error; err != nil {
		return nil, err
	}

	movieRows := make([]model.Media, 0, len(rows))
	episodeRows := make([]model.Media, 0, len(rows))
	for i := range rows {
		if e.mediaShouldBeEpisode(ctx, &rows[i]) {
			episodeRows = append(episodeRows, rows[i])
		} else {
			movieRows = append(movieRows, rows[i])
		}
	}

	entries := make([]embyTopLevelEntry, 0, len(movieRows)+len(episodeRows))
	if includeMovies && len(movieRows) > 0 {
		payloads, err := e.payloadsForMedia(ctx, movieRows, p.UserID)
		if err != nil {
			return nil, err
		}
		for i, payload := range payloads {
			row := movieRows[i]
			entries = append(entries, embyTopLevelEntry{
				Payload:   payload,
				Name:      row.Title,
				CreatedAt: row.CreatedAt,
				Year:      row.Year,
				Rating:    row.Rating,
			})
		}
	}
	if includeSeries && len(episodeRows) > 0 {
		groups := e.seriesGroupsFromMedia(episodeRows)
		for _, group := range groups {
			entries = append(entries, embyTopLevelEntry{
				Payload:   e.seriesPayload(group),
				Name:      group.Name,
				CreatedAt: group.CreatedAt,
				Year:      group.Year,
				Rating:    group.Rating,
			})
		}
	}

	sortTopLevelEntries(entries, p)
	total := len(entries)
	paged := pageSlice(entries, p.StartIndex, p.Limit)
	items := make([]map[string]any, 0, len(paged))
	for _, entry := range paged {
		items = append(items, entry.Payload)
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) movieItemsForLibrary(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	rows, err := e.mediaRowsForLibraryAutoClassification(ctx, libraryID, p)
	if err != nil {
		return nil, err
	}
	movieRows := make([]model.Media, 0, len(rows))
	for i := range rows {
		if !e.mediaShouldBeEpisode(ctx, &rows[i]) {
			movieRows = append(movieRows, rows[i])
		}
	}
	total := len(movieRows)
	items, err := e.payloadsForMedia(ctx, pageSlice(movieRows, p.StartIndex, p.Limit), p.UserID)
	if err != nil {
		return nil, err
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) episodeItemsForLibrary(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	rows, err := e.mediaRowsForLibraryAutoClassification(ctx, libraryID, p)
	if err != nil {
		return nil, err
	}
	return e.episodeItems(ctx, rows, p)
}

func (e *EmbyService) mediaRowsForLibraryAutoClassification(ctx context.Context, libraryID string, p ItemsParams) ([]model.Media, error) {
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if p.SearchTerm != "" {
		q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
	}
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return []model.Media{}, nil
		}
		q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
	}
	resumeFilter := containsEmbyFilter(p.Filters, "IsResumable")
	if resumeFilter {
		if strings.TrimSpace(p.UserID) == "" {
			return []model.Media{}, nil
		}
		q = q.Joins(`JOIN (
			SELECT media_id, MAX(watched_at) AS watched_at
			FROM playback_histories
			WHERE user_id = ? AND completed = ? AND position_ms > 0
			GROUP BY media_id
		) AS resume ON resume.media_id = media.id`, p.UserID, false)
	}
	var rows []model.Media
	if err := q.Order(embyMediaOrderClause(p.SortBy, p.SortOrder, resumeFilter)).Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func embyMediaOrderClause(sortBy, sortOrder string, resumeFilter bool) string {
	order := mediaReleaseOrderSQL(true)
	orderIncludesDirection := true
	switch primarySupportedEmbySort(sortBy, resumeFilter) {
	case "sortname", "name":
		order = "media.title"
		orderIncludesDirection = false
	case "premieredate", "productionyear":
		desc := !strings.EqualFold(firstCSVValue(sortOrder), "Ascending")
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
	if !orderIncludesDirection && strings.EqualFold(firstCSVValue(sortOrder), "Descending") {
		if !strings.HasSuffix(order, " desc") {
			order += " desc"
		}
	}
	return order
}

func sortTopLevelEntries(entries []embyTopLevelEntry, p ItemsParams) {
	sortBy := primarySupportedEmbySort(p.SortBy, false)
	desc := strings.EqualFold(firstCSVValue(p.SortOrder), "Descending")
	if sortBy == "" {
		sortBy = "datecreated"
		desc = true
	}
	sort.SliceStable(entries, func(i, j int) bool {
		switch sortBy {
		case "sortname", "name":
			left := strings.ToLower(entries[i].Name)
			right := strings.ToLower(entries[j].Name)
			if left != right {
				if desc {
					return left > right
				}
				return left < right
			}
		case "premieredate", "productionyear":
			if entries[i].Year != entries[j].Year {
				if desc {
					return entries[i].Year > entries[j].Year
				}
				return entries[i].Year < entries[j].Year
			}
		case "communityrating":
			if entries[i].Rating != entries[j].Rating {
				if desc {
					return entries[i].Rating > entries[j].Rating
				}
				return entries[i].Rating < entries[j].Rating
			}
		}
		if desc {
			return entries[i].CreatedAt.After(entries[j].CreatedAt)
		}
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
}

func (e *EmbyService) filterEpisodeRows(ctx context.Context, rows []model.Media) []model.Media {
	if len(rows) == 0 {
		return rows
	}
	out := rows[:0]
	for i := range rows {
		if e.mediaShouldBeEpisode(ctx, &rows[i]) {
			out = append(out, rows[i])
		}
	}
	return out
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
