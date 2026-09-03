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
	if hasEmbyGenreFilter(p) {
		var rows []model.Media
		if err := q.Order(order).Find(&rows).Error; err != nil {
			return nil, err
		}
		rows = e.filterMediaRowsByEmbyGenres(rows, p)
		if collapseVersions {
			rows = e.collapseMediaVersionRows(ctx, rows)
			if primarySupportedEmbySort(p.SortBy, resumeFilter) == "datecreated" {
				sortEmbyMediaRowsByDateCreated(rows, desc)
			}
		}
		total := int64(len(rows))
		rows = pageSlice(rows, p.StartIndex, p.Limit)
		items, err := e.payloadsForMediaRows(ctx, rows, p.UserID, !p.OmitMediaSources, !collapseVersions)
		if err != nil {
			return nil, err
		}
		out := map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}
		if e.cache != nil {
			e.cache.SetJSON(ctx, cacheKey, embyItemsCacheValue{Items: items, TotalRecordCount: total, StartIndex: p.StartIndex}, e.embyMediaCacheTTL())
		}
		return out, nil
	}
	if collapseVersions {
		// Pagination applies to logical Emby items, not physical media rows.
		// Every matching row must participate in version grouping so both the
		// requested page and TotalRecordCount describe the same result set.
		var rows []model.Media
		if err := q.Order(order).Find(&rows).Error; err != nil {
			return nil, err
		}
		rows = e.collapseMediaVersionRows(ctx, rows)
		if primarySupportedEmbySort(p.SortBy, resumeFilter) == "datecreated" {
			sortEmbyMediaRowsByDateCreated(rows, desc)
		}
		total := int64(len(rows))
		rows = pageSlice(rows, p.StartIndex, p.Limit)
		// rows already represent the public logical items. Collapsing again here
		// can shrink a non-final page after its offset was chosen, making the
		// next client request overlap this page.
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
	rows = e.filterMediaRowsForUser(ctx, rows, p.UserID)
	if embyHasMediaSearch(p) {
		filtered := rows[:0]
		for _, row := range rows {
			if embyMediaMatchesSearch(row, p) {
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
	rows = e.collapseMediaVersionRows(ctx, rows)
	total := len(rows)
	items, err := e.payloadsForMediaRows(ctx, pageSlice(rows, p.StartIndex, p.Limit), p.UserID, !p.OmitMediaSources, false)
	if err != nil {
		return nil, err
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

func (e *EmbyService) payloadsForMedia(ctx context.Context, rows []model.Media, userID string, includeMediaSources bool) ([]map[string]any, error) {
	return e.payloadsForMediaRows(ctx, rows, userID, includeMediaSources, true)
}

func (e *EmbyService) payloadsForMediaRows(ctx context.Context, rows []model.Media, userID string, includeMediaSources, collapseVersions bool) ([]map[string]any, error) {
	if collapseVersions {
		rows = e.collapseMediaVersionRows(ctx, rows)
	}
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

	items := make([]map[string]any, 0, len(rows))
	for _, m := range rows {
		items = append(items, e.itemPayloadWithOptions(ctx, &m, userFavs[m.ID], userPos[m.ID], includeMediaSources, userWatchedAt[m.ID]))
	}
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
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("season_num > 0 OR episode_num > 0").
		Where("COALESCE(part_group_key, '') = ''")
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if libraryID != "" {
		q = q.Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	}
	q = applyEmbyMediaSearch(q, p)
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
	rows = e.filterMediaRowsByEmbyGenres(rows, p)
	groups := e.seriesGroupsFromMedia(rows)
	partQ := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("COALESCE(part_group_key, '') <> ''")
	partQ = e.applyUserMediaVisibility(ctx, partQ, p.UserID)
	if libraryID != "" {
		partQ = partQ.Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	}
	partQ = applyEmbyMediaSearch(partQ, p)
	var multipartRows []model.Media
	if err := partQ.Order("media.part_group_key ASC, media.part_index ASC, media.created_at ASC").
		Limit(embySeriesGroupingLimit).Find(&multipartRows).Error; err != nil {
		return nil, err
	}
	multipartRows = e.filterMediaRowsByEmbyGenres(multipartRows, p)
	groups = append(groups, e.multipartSeriesGroupsFromMedia(multipartRows)...)
	sortSeriesGroups(groups, p)
	total := len(groups)
	items := make([]map[string]any, 0, minInt(p.Limit, len(groups)))
	for _, group := range pageSlice(groups, p.StartIndex, p.Limit) {
		items = append(items, e.seriesPayload(group))
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}
