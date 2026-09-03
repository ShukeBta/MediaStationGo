package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const embyResumeItemsLimit = 10

// Item 单条目详情。
func (e *EmbyService) Item(ctx context.Context, mediaID, userID string) (map[string]any, error) {
	if personName, ok := embyPersonName(mediaID); ok {
		return e.personItem(ctx, userID, personName)
	}
	if genreName, ok := embyGenreName(mediaID); ok {
		return e.genreItem(ctx, userID, genreName)
	}
	if lib, err := e.repo.Library.FindByID(ctx, mediaID); err != nil {
		return nil, err
	} else if lib != nil {
		libs := FilterDisplayCloudLibraries(ctx, e.repo, []model.Library{*lib})
		if len(libs) == 0 {
			return nil, nil
		}
		visibility := e.mediaVisibility(ctx, userID)
		if !e.libraryVisibleFromCachedVisibility(libs[0], visibility) {
			return nil, nil
		}
		return e.libraryAsView(ctx, &libs[0]), nil
	}
	if strings.HasPrefix(mediaID, embyVirtualSeasonPrefix) {
		if season, ok, err := e.findSeasonGroup(ctx, mediaID, userID); err != nil {
			return nil, err
		} else if ok {
			return e.seasonPayload(season), nil
		}
	}
	if strings.HasPrefix(mediaID, embyVirtualSeriesPrefix) {
		if series, ok, err := e.findSeriesGroup(ctx, mediaID, userID); err != nil {
			return nil, err
		} else if ok {
			return e.seriesPayload(series), nil
		}
	}
	m, err := e.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		if series, ok, err := e.findSeriesGroup(ctx, mediaID, userID); err != nil {
			return nil, err
		} else if ok {
			return e.seriesPayload(series), nil
		}
		return nil, nil
	}
	if !UserDefaultMediaVisibility(ctx, e.repo, userID).Allows(m) {
		return nil, nil
	}
	if strings.TrimSpace(m.PartGroupKey) != "" {
		view := e.multipartEpisodeForMedia(ctx, *m)
		m = &view
	}
	fav := false
	pos := int64(0)
	watchedAt := time.Time{}
	if userID != "" {
		var f model.Favorite
		ferr := e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id = ?", userID, mediaID).First(&f).Error
		if ferr == nil {
			fav = true
		}
		var h model.PlaybackHistory
		herr := e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id = ?", userID, mediaID).
			Order("watched_at DESC, updated_at DESC, id DESC").First(&h).Error
		if herr == nil {
			pos = h.PositionMs
			watchedAt = h.WatchedAt
		}
	}
	item := e.itemPayload(ctx, m, fav, pos, watchedAt)
	if err := e.attachExternalSubtitleStreamsToItem(ctx, item, m); err != nil {
		return nil, err
	}
	return item, nil
}

// LatestItems 最近添加，全库或指定库。
func (e *EmbyService) LatestItems(ctx context.Context, userID, parentID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	cacheKey := e.embyLatestCacheKey(userID, parentID, limit)
	var cached embyLatestCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		return cached.Items, nil
	}
	if e.cache != nil {
		call, owner := e.beginEmbyReadCacheFill(cacheKey)
		if !owner {
			if err := waitEmbyReadCacheFill(ctx, call); err != nil {
				return nil, err
			}
			if e.cache.GetJSON(ctx, cacheKey, &cached) {
				return cached.Items, nil
			}
		} else {
			defer e.finishEmbyReadCacheFill(cacheKey, call)
		}
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("deleted_at IS NULL")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if parentID != "" {
		if episodic, err := e.libraryIsEpisodic(ctx, parentID); err == nil && episodic {
			out, err := e.latestSeriesItemsForLibrary(ctx, userID, parentID, limit)
			if err == nil && e.cache != nil {
				e.cache.SetJSON(ctx, cacheKey, embyLatestCacheValue{Items: out}, e.embyMediaCacheTTL())
			}
			return out, err
		}
		q = q.Where("library_id IN ?", e.mergedLibraryIDs(ctx, parentID))
	}
	rowLimit := limit * 4
	if rowLimit < 100 {
		rowLimit = 100
	}
	if rowLimit > 500 {
		rowLimit = 500
	}
	var rows []model.Media
	// 「最近添加」= 按入库时间倒序，与 Emby /Items/Latest 语义一致。
	if err := q.Order("media.created_at DESC, media.id DESC").Limit(rowLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	rows = e.collapseMediaVersionRows(ctx, rows)
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out, err := e.payloadsForMedia(ctx, rows, userID, true)
	if err != nil {
		return nil, err
	}
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyLatestCacheValue{Items: out}, e.embyMediaCacheTTL())
	}
	return out, nil
}

func (e *EmbyService) latestSeriesItemsForLibrary(ctx context.Context, userID, libraryID string, limit int) ([]map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ? AND (season_num > 0 OR episode_num > 0)", e.mergedLibraryIDs(ctx, libraryID))
	q = e.applyUserMediaVisibility(ctx, q, userID)
	var rows []model.Media
	if err := q.Order("media.created_at DESC, media.id DESC").Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	groups := e.seriesGroupsFromMedia(rows)
	sortSeriesGroups(groups, ItemsParams{SortBy: "datecreated", SortOrder: "Descending"})
	if len(groups) > limit {
		groups = groups[:limit]
	}
	items := make([]map[string]any, 0, len(groups))
	for _, group := range groups {
		items = append(items, e.seriesPayload(group))
	}
	return items, nil
}

// ResumeItems 列出有未完成播放进度的媒体。
func (e *EmbyService) ResumeItems(ctx context.Context, userID string) (map[string]any, error) {
	out, err := e.ResumeItemsPage(ctx, userID, 0, embyResumeItemsLimit)
	if err != nil {
		return nil, err
	}
	if items, ok := out["Items"].([]map[string]any); ok {
		out["TotalRecordCount"] = len(items)
	}
	return out, nil
}

// ResumeItemsPage 按 Emby 的 StartIndex/Limit 语义分页续播项。
func (e *EmbyService) ResumeItemsPage(ctx context.Context, userID string, startIndex, limit int) (map[string]any, error) {
	if startIndex < 0 {
		startIndex = 0
	}
	if limit <= 0 || limit > 500 {
		limit = embyResumeItemsLimit
	}
	var hist []model.PlaybackHistory
	if err := e.repo.DB.WithContext(ctx).
		Where("user_id = ? AND completed = ? AND position_ms > 0", userID, false).
		Where("NOT EXISTS (SELECT 1 FROM user_media_playback_preferences p WHERE p.user_id = ? AND p.media_id = playback_histories.media_id AND p.hidden_from_resume = ? AND p.deleted_at IS NULL)", userID, true).
		Order("watched_at DESC, updated_at DESC, id DESC").Find(&hist).Error; err != nil {
		return nil, err
	}
	if len(hist) == 0 {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": startIndex}, nil
	}
	ids := make([]string, 0, len(hist))
	histByID := map[string]model.PlaybackHistory{}
	for _, h := range hist {
		mediaID := strings.TrimSpace(h.MediaID)
		if mediaID == "" {
			continue
		}
		if _, exists := histByID[mediaID]; exists {
			continue
		}
		ids = append(ids, mediaID)
		histByID[mediaID] = h
	}
	if len(ids) == 0 {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": startIndex}, nil
	}
	var medias []model.Media
	q := e.repo.DB.WithContext(ctx).Where("id IN ?", ids)
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if err := q.Find(&medias).Error; err != nil {
		return nil, err
	}
	byID := map[string]*model.Media{}
	for i := range medias {
		byID[medias[i].ID] = &medias[i]
	}
	visibleIDs := make([]string, 0, len(ids))
	seenLogicalItems := make(map[string]struct{})
	for _, id := range ids {
		media, ok := byID[id]
		if !ok {
			continue
		}
		logicalKey := id
		if key := mediaPartGroupKey(*media); key != "" {
			logicalKey = key
		}
		if _, duplicate := seenLogicalItems[logicalKey]; duplicate {
			continue
		}
		seenLogicalItems[logicalKey] = struct{}{}
		visibleIDs = append(visibleIDs, id)
	}
	pageIDs := pageSlice(visibleIDs, startIndex, limit)
	items := make([]map[string]any, 0, len(pageIDs))
	for _, id := range pageIDs {
		h := histByID[id]
		items = append(items, e.itemPayload(ctx, byID[id], false, h.PositionMs, h.WatchedAt))
	}
	if err := e.attachResumeSeriesArtwork(ctx, userID, items); err != nil {
		return nil, err
	}
	return map[string]any{"Items": items, "TotalRecordCount": len(visibleIDs), "StartIndex": startIndex}, nil
}

func (e *EmbyService) itemPayload(ctx context.Context, m *model.Media, fav bool, posMs int64, watchedAtValues ...time.Time) map[string]any {
	return e.itemPayloadWithOptions(ctx, m, fav, posMs, true, watchedAtValues...)
}

func (e *EmbyService) itemPayloadWithOptions(ctx context.Context, m *model.Media, fav bool, posMs int64, includeMediaSources bool, watchedAtValues ...time.Time) map[string]any {
	itemType := "Movie"
	name := m.Title
	parentID := m.LibraryID
	seriesID := m.SeriesID
	seriesName := ""
	seasonID := ""
	partCount := 0
	if strings.TrimSpace(m.PartGroupKey) != "" {
		name = firstNonEmpty(m.PartGroupTitle, m.Title)
		partCount = m.PartCount
		if partCount <= 0 {
			partCount = e.mediaPartCount(ctx, m)
		}
	}
	if e.mediaShouldBeEpisode(ctx, m) {
		itemType = "Episode"
		seriesID = e.seriesIDForMedia(m)
		seriesName = e.seriesNameForMedia(m)
		seasonID = e.seasonIDForMedia(m)
		parentID = seasonID
		episodeTitle := strings.TrimSpace(m.EpisodeTitle)
		if episodeTitle != "" {
			name = episodeTitle
		} else if m.EpisodeNum > 0 {
			name = fmt.Sprintf("第 %d 集", m.EpisodeNum)
		}
	}
	name = e.adultDisplayName(ctx, m, name)
	imageTags := map[string]string{}
	backdropTags := []string{}
	primaryArtwork := e.mediaPrimaryArtwork(ctx, m)
	backdropArtwork := e.mediaBackdropArtwork(ctx, m)
	if primaryArtwork != "" {
		imageTags["Primary"] = embyImageTag(m.ID, "primary", primaryArtwork, m.UpdatedAt)
	}
	if backdropArtwork != "" {
		backdropTags = append(backdropTags, embyImageTag(m.ID, "backdrop", backdropArtwork, m.UpdatedAt))
	}
	// Emby 单集卡片普遍优先取 Thumb（横版剧照）；不暴露它会回落到父剧集的
	// 背景图，导致“最近观看/NextUp”里每一集的封面都长一样。
	// 剧照仍保存在 BackdropURL，Backdrop 契约维持不变（Primary=剧照）。
	if e.mediaShouldBeEpisode(ctx, m) && strings.TrimSpace(m.BackdropURL) != "" {
		imageTags["Thumb"] = embyImageTag(m.ID, "thumb", m.BackdropURL, m.UpdatedAt)
	}
	modifiedAt := m.UpdatedAt
	if modifiedAt.IsZero() {
		modifiedAt = m.CreatedAt
	}

	runTimeTicks := int64(m.DurationSec) * 10_000_000
	durationMs := int64(m.DurationSec) * 1000
	played := posMs > 0 && durationMs > 0 && posMs >= durationMs*9/10
	pct := 0.0
	if durationMs > 0 {
		pct = float64(posMs) / float64(durationMs) * 100
	}
	userData := map[string]any{
		"PlaybackPositionTicks": posMs * 10_000,
		"PlayCount":             0,
		"IsFavorite":            fav,
		"Played":                played,
		"PlayedPercentage":      pct,
	}
	if len(watchedAtValues) > 0 && !watchedAtValues[0].IsZero() {
		userData["LastPlayedDate"] = watchedAtValues[0].UTC().Format(time.RFC3339Nano)
	}
	people := e.embyPeopleFromCSV(ctx, m.Actors)
	genres := e.embyGenresForMedia(m, "")

	item := map[string]any{
		"Id":                m.ID,
		"Name":              name,
		"OriginalTitle":     m.OriginalName,
		"ServerId":          embyServerID,
		"Type":              itemType,
		"MediaType":         "Video",
		"IsFolder":          false,
		"ProductionYear":    m.Year,
		"ParentIndexNumber": m.SeasonNum,
		"IndexNumber":       m.EpisodeNum,
		"Overview":          m.Overview,
		"RunTimeTicks":      runTimeTicks,
		"CommunityRating":   m.Rating,
		"Container":         m.Container,
		"Width":             m.Width,
		"Height":            m.Height,
		"DateCreated":       m.CreatedAt,
		"DateModified":      modifiedAt,
		"Etag":              embyItemETag(m.ID, modifiedAt, name, m.OriginalName, primaryArtwork, backdropArtwork, m.Path, m.Actors, embyPeopleImageSignature(people), m.PartGroupKey, fmt.Sprintf("%d", partCount)),
		"Path":              m.Path,
		"ParentId":          parentID,
		"SeasonId":          seasonID,
		"SeasonName":        seasonName(m.SeasonNum),
		"SeriesId":          seriesID,
		"SeriesName":        seriesName,
		"ImageTags":         imageTags,
		"BackdropImageTags": backdropTags,
		"Genres":            genres,
		"GenreItems":        embyGenreItems(genres),
		"People":            people,
		"ProviderIds": map[string]string{
			"Tmdb":    intToStr(m.TMDbID),
			"Bangumi": intToStr(m.BangumiID),
		},
		"UserData": userData,
	}
	if includeMediaSources {
		item["MediaSources"] = e.mediaSourcesForItem(ctx, m, true, false)
	}
	if itemType == "Movie" && partCount > 1 {
		item["PartCount"] = partCount
	}
	if ratio, ok := e.primaryImageAspectRatio(ctx, m, primaryArtwork); ok {
		item["PrimaryImageAspectRatio"] = ratio
	}
	if itemType == "Movie" {
		for _, key := range []string{"SeasonId", "SeasonName", "SeriesId", "SeriesName", "ParentIndexNumber", "IndexNumber"} {
			delete(item, key)
		}
	}
	if premiered, ok := embyPremiereDate(m.ReleaseDate); ok {
		item["PremiereDate"] = premiered
	}
	embyAttachImageOwnerIDs(item)
	return item
}

func (e *EmbyService) primaryImageAspectRatio(ctx context.Context, m *model.Media, primaryArtwork string) (float64, bool) {
	if m == nil || strings.TrimSpace(primaryArtwork) == "" {
		return 0, false
	}
	if e.mediaShouldBeEpisode(ctx, m) && strings.TrimSpace(m.BackdropURL) != "" {
		// TMDb episode stills are landscape artwork. The media stream dimensions
		// describe the video, not the image, and can make generic Emby clients
		// reserve an incorrect poster shape for cropped or ultrawide content.
		return 16.0 / 9.0, true
	}
	return 2.0 / 3.0, true
}
