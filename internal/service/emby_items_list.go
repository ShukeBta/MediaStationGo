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
	if e.cache != nil {
		call, owner := e.beginEmbyReadCacheFill(cacheKey)
		if !owner {
			if err := waitEmbyReadCacheFill(ctx, call); err != nil {
				return nil, err
			}
			if e.cache.GetJSON(ctx, cacheKey, &cached) {
				return map[string]any{"Items": cached.Items, "TotalRecordCount": cached.TotalRecordCount, "StartIndex": cached.StartIndex}, nil
			}
		} else {
			defer e.finishEmbyReadCacheFill(cacheKey, call)
		}
	}
	var err error
	ctx, err = e.withEmbyLibrarySnapshot(ctx)
	if err != nil {
		return nil, err
	}
	// 列表查询不展示搜索拼音/首字母，这两个 text 字段平均各数百字节；Omit 掉
	// 避免 SELECT * 把 907 行的大字段全拉进内存再反射映射，显著降低 /Items 耗时。
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Omit("search_pinyin", "search_initials")
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if p.ParentID != "" {
		q = q.Where("library_id IN ? OR series_id = ?", e.mergedLibraryIDs(ctx, p.ParentID), p.ParentID)
	}
	q = applyEmbyMediaSearch(q, p)
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
			SELECT ph.media_id, MAX(ph.watched_at) AS watched_at
			FROM playback_histories ph
			WHERE ph.user_id = ? AND ph.completed = ? AND ph.position_ms > 0
			  AND NOT EXISTS (
			    SELECT 1 FROM user_media_playback_preferences p
			    WHERE p.user_id = ph.user_id AND p.media_id = ph.media_id
			      AND p.hidden_from_resume = ? AND p.deleted_at IS NULL
			  )
			GROUP BY ph.media_id
		) AS resume ON resume.media_id = media.id`, p.UserID, false, true)
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
		// Offset pagination needs a total order. CreatedAt alone is not unique
		// for scanner batches, so use the public Emby media Id as a same-direction
		// tie-breaker instead of letting the database choose an arbitrary row order.
		if desc {
			order = "media.created_at DESC, media.id DESC"
		} else {
			order = "media.created_at ASC, media.id ASC"
		}
		orderIncludesDirection = true
	case "dateplayed":
		order = embyDatePlayedOrder(desc)
		orderIncludesDirection = true
	case "communityrating":
		order = "media.rating"
		orderIncludesDirection = false
	}
	if !orderIncludesDirection && strings.EqualFold(firstCSVValue(p.SortOrder), "Descending") {
		if !strings.HasSuffix(order, " desc") {
			order = order + " desc"
		}
	}

	collapseVersions := e.shouldCollapseMediaVersions(ctx, p)
	if collapseVersions || hasEmbyGenreFilter(p) {
		if err := e.ensureEmbyKeys(ctx, q); err != nil {
			return nil, err
		}
	}
	if hasEmbyGenreFilter(p) {
		q = applyEmbyBrowseGenres(q, p)
	}
	if collapseVersions {
		rows, total, err := e.collapsedMediaPageSQL(ctx, q, p, order, resumeFilter, desc)
		if err != nil {
			return nil, err
		}
		items, err := e.payloadsForMediaRows(ctx, rows, p.UserID, !p.OmitMediaSources, false)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}
		if e.cache != nil {
			e.cache.SetJSON(ctx, cacheKey, embyItemsCacheValue{Items: items, TotalRecordCount: total, StartIndex: p.StartIndex}, e.embyMediaCacheTTL())
		}
		return out, nil
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, err
	}
	var rows []model.Media
	if err := q.Order(order).Offset(p.StartIndex).Limit(p.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	items, err := e.payloadsForMedia(ctx, rows, p.UserID, !p.OmitMediaSources)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyItemsCacheValue{Items: items, TotalRecordCount: total, StartIndex: p.StartIndex}, e.embyMediaCacheTTL())
	}
	return out, nil
}

func (e *EmbyService) episodeItems(ctx context.Context, rows []model.Media, p ItemsParams) (map[string]any, error) {
	// Version identity uses merged libraries for every row. Load their facts
	// once before filtering/collapse and reuse them during payload assembly.
	snapshotDone := MeasureEpisodeStage(ctx, "library_snapshot")
	var err error
	ctx, err = e.withEmbyLibrarySnapshot(ctx)
	snapshotDone()
	if err != nil {
		return nil, err
	}
	filterDone := MeasureEpisodeStage(ctx, "episode_filter_sort")
	// 混合库里只保留真正的剧集行(yebuwudong 虚拟"剧集"视图)。
	rows = e.filterEpisodeRows(ctx, rows)
	rows = e.filterMediaRowsForUser(ctx, rows, p.UserID)
	rows, err = e.filterMediaRowsByPeople(ctx, rows, p.PersonIDs)
	if err != nil {
		return nil, err
	}
	if embyHasMediaSearch(p) {
		searchParams := p
		searchParams.PersonIDs = nil // The database filter above includes structured crew credits.
		filtered := rows[:0]
		for _, row := range rows {
			if embyMediaMatchesSearch(row, searchParams) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}
	rows = e.filterMediaRowsByEmbyGenres(rows, p)
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].SeasonNum != rows[j].SeasonNum {
			return rows[i].SeasonNum < rows[j].SeasonNum
		}
		if rows[i].EpisodeNum != rows[j].EpisodeNum {
			return rows[i].EpisodeNum < rows[j].EpisodeNum
		}
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
	// Pagination must describe logical Emby episodes. Collapsing physical
	// versions after slicing can make the reported total disagree with the
	// returned items and repeat the same representative on a later page.
	filterDone()
	collapseDone := MeasureEpisodeStage(ctx, "episode_version_collapse")
	rows = e.collapseMediaVersionRows(ctx, rows)
	collapseDone()
	total := len(rows)
	items, err := e.payloadsForMediaRows(ctx, pageSlice(rows, p.StartIndex, p.Limit), p.UserID, !p.OmitMediaSources, false)
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
		payloads, err := e.payloadsForMedia(ctx, movieRows, p.UserID, !p.OmitMediaSources)
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
		groups, err := e.seriesGroupsFromMedia(ctx, episodeRows)
		if err != nil {
			return nil, err
		}
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
	items, err := e.payloadsForMedia(ctx, pageSlice(movieRows, p.StartIndex, p.Limit), p.UserID, !p.OmitMediaSources)
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

func (e *EmbyService) payloadsForMedia(ctx context.Context, rows []model.Media, userID string, includeMediaSources bool) ([]map[string]any, error) {
	return e.payloadsForMediaRows(ctx, rows, userID, includeMediaSources, true)
}

func (e *EmbyService) payloadsForMediaRows(ctx context.Context, rows []model.Media, userID string, includeMediaSources, collapseVersions bool) ([]map[string]any, error) {
	ctx = e.withPersonCredits(ctx, rows)
	var err error
	done := MeasureEpisodeStage(ctx, "library_snapshot")
	ctx, err = e.withEmbyLibrarySnapshot(ctx)
	done()
	if err != nil {
		return nil, err
	}
	if collapseVersions {
		rows = e.collapseMediaVersionRows(ctx, rows)
	}
	done = MeasureEpisodeStage(ctx, "series_titles")
	ctx, err = e.withEmbySeriesTitles(ctx, rows)
	done()
	if err != nil {
		return nil, err
	}
	if includeMediaSources {
		done = MeasureEpisodeStage(ctx, "media_version_siblings")
		ctx, err = e.withEmbyMediaVersionSiblings(ctx, rows)
		done()
		if err != nil {
			return nil, err
		}
	}
	done = MeasureEpisodeStage(ctx, "user_favorites_history")
	userFavs := map[string]bool{}
	userPos := map[string]int64{}
	userWatchedAt := map[string]time.Time{}
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
		histQuery := e.repo.DB.WithContext(ctx).Where("user_id = ?", userID).Where("media_id IN ?", mediaIDs).
			Order("watched_at DESC, updated_at DESC, id DESC")
		_ = histQuery.Find(&hist).Error
		for _, h := range hist {
			if _, exists := userPos[h.MediaID]; !exists {
				userPos[h.MediaID] = h.PositionMs
				userWatchedAt[h.MediaID] = h.WatchedAt
			}
		}
	}

	done()
	done = MeasureEpisodeStage(ctx, "item_payload_assembly")
	items := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		items = append(items, e.itemPayloadWithOptions(ctx, &m, userFavs[m.ID], userPos[m.ID], includeMediaSources, userWatchedAt[m.ID]))
	}
	done()
	return items, nil
}

func (e *EmbyService) shouldCollapseMediaVersions(ctx context.Context, p ItemsParams) bool {
	if (containsItemType(p.IncludeItemTypes, "Series") || containsItemType(p.IncludeItemTypes, "Season")) &&
		!containsItemType(p.IncludeItemTypes, "Movie") &&
		!containsItemType(p.IncludeItemTypes, "Episode") {
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

func sortEmbyMediaRowsByDateCreated(rows []model.Media, descending bool) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			if descending {
				return rows[i].ID > rows[j].ID
			}
			return rows[i].ID < rows[j].ID
		}
		if descending {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].CreatedAt.Before(rows[j].CreatedAt)
	})
}

func (e *EmbyService) collapseMediaVersionRows(ctx context.Context, rows []model.Media) []model.Media {
	if len(rows) < 2 {
		return rows
	}
	out := make([]model.Media, 0, len(rows))
	indexByKey := make(map[string]int, len(rows))
	for _, row := range rows {
		if partKey := mediaPartGroupKey(row); partKey != "" {
			if row.SeasonNum > 0 || row.EpisodeNum > 0 {
				out = append(out, row)
				continue
			}
			if idx, ok := indexByKey[partKey]; ok {
				partCount := out[idx].PartCount + 1
				if betterMediaPart(row, out[idx]) {
					row.PartCount = partCount
					out[idx] = row
				} else {
					out[idx].PartCount = partCount
				}
				continue
			}
			row.PartCount = 1
			indexByKey[partKey] = len(out)
			out = append(out, row)
			continue
		}
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
	cacheKey := e.embySeriesCacheKey(ctx, p)
	var cached embyItemsCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		return map[string]any{"Items": cached.Items, "TotalRecordCount": cached.TotalRecordCount, "StartIndex": cached.StartIndex}, nil
	}
	if e.cache != nil {
		call, owner := e.beginEmbyReadCacheFill(cacheKey)
		if !owner {
			if err := waitEmbyReadCacheFill(ctx, call); err != nil {
				return nil, err
			}
			if e.cache.GetJSON(ctx, cacheKey, &cached) {
				return map[string]any{"Items": cached.Items, "TotalRecordCount": cached.TotalRecordCount, "StartIndex": cached.StartIndex}, nil
			}
		} else {
			defer e.finishEmbyReadCacheFill(cacheKey, call)
		}
	}

	out, err := e.seriesPageSQL(ctx, libraryID, p)
	if err != nil {
		return nil, err
	}
	items, _ := out["Items"].([]map[string]any)
	total, _ := out["TotalRecordCount"].(int)
	start, _ := out["StartIndex"].(int)
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyItemsCacheValue{
			Items: items, TotalRecordCount: int64(total), StartIndex: start,
		}, e.embySeriesCacheTTL(p))
	}
	return out, nil
}
