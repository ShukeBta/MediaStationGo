package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// Item 单条目详情。
func (e *EmbyService) Item(ctx context.Context, mediaID, userID string) (map[string]any, error) {
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
		return e.libraryAsView(&libs[0]), nil
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
	fav := false
	pos := int64(0)
	if userID != "" {
		var f model.Favorite
		ferr := e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id = ?", userID, mediaID).First(&f).Error
		if ferr == nil {
			fav = true
		}
		var h model.PlaybackHistory
		herr := e.repo.DB.WithContext(ctx).Where("user_id = ? AND media_id = ?", userID, mediaID).
			Order("watched_at desc").First(&h).Error
		if herr == nil {
			pos = h.PositionMs
		}
	}
	return e.itemPayload(ctx, m, fav, pos), nil
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
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("deleted_at IS NULL")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if parentID != "" {
		if episodic, err := e.libraryIsEpisodic(ctx, parentID); err == nil && episodic {
			out, err := e.latestSeriesItemsForLibrary(ctx, userID, parentID, limit)
			if err == nil && e.cache != nil {
				e.cache.SetJSON(ctx, cacheKey, embyLatestCacheValue{Items: out}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
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
	if err := q.Order(mediaReleaseOrderSQL(true)).Limit(rowLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	rows = e.collapseMediaVersionRows(ctx, rows)
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out, err := e.payloadsForMedia(ctx, rows, userID)
	if err != nil {
		return nil, err
	}
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyLatestCacheValue{Items: out}, time.Duration(e.mediaCacheTTLSeconds())*time.Second)
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
	if err := q.Order(mediaReleaseOrderSQL(true)).Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	groups := e.seriesGroupsFromMedia(rows)
	sortSeriesGroups(groups, ItemsParams{SortBy: "premieredate", SortOrder: "Descending"})
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
func (e *EmbyService) ResumeItems(ctx context.Context, userID string, limit int) (map[string]any, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var hist []model.PlaybackHistory
	if err := e.repo.DB.WithContext(ctx).
		Where("user_id = ? AND completed = ? AND position_ms > 0", userID, false).
		Order("watched_at desc").Limit(limit).Find(&hist).Error; err != nil {
		return nil, err
	}
	if len(hist) == 0 {
		return map[string]any{"Items": []any{}, "TotalRecordCount": 0}, nil
	}
	ids := make([]string, 0, len(hist))
	posByID := map[string]int64{}
	for _, h := range hist {
		ids = append(ids, h.MediaID)
		posByID[h.MediaID] = h.PositionMs
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
	items := make([]map[string]any, 0, len(hist))
	for _, h := range hist {
		if m, ok := byID[h.MediaID]; ok {
			items = append(items, e.itemPayload(ctx, m, false, posByID[h.MediaID]))
		}
	}
	return map[string]any{"Items": items, "TotalRecordCount": len(items)}, nil
}

func (e *EmbyService) itemPayload(ctx context.Context, m *model.Media, fav bool, posMs int64) map[string]any {
	itemType := "Movie"
	name := m.Title
	parentID := m.LibraryID
	seriesID := m.SeriesID
	seriesName := ""
	seasonID := ""
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
	imageTags := map[string]string{}
	backdropTags := []string{}
	primaryArtwork := e.mediaPrimaryArtwork(ctx, m)
	backdropArtwork := e.mediaBackdropArtwork(ctx, m)
	if primaryArtwork != "" {
		imageTags["Primary"] = m.ID
	}
	if backdropArtwork != "" {
		backdropTags = append(backdropTags, m.ID+"-bd")
	}

	runTimeTicks := int64(m.DurationSec) * 10_000_000
	durationMs := int64(m.DurationSec) * 1000
	played := posMs > 0 && durationMs > 0 && posMs >= durationMs*9/10
	pct := 0.0
	if durationMs > 0 {
		pct = float64(posMs) / float64(durationMs) * 100
	}

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
		"Path":              m.Path,
		"ParentId":          parentID,
		"SeasonId":          seasonID,
		"SeasonName":        seasonName(m.SeasonNum),
		"SeriesId":          seriesID,
		"SeriesName":        seriesName,
		"ImageTags":         imageTags,
		"BackdropImageTags": backdropTags,
		"Genres":            splitCSV(m.Genres),
		"ProviderIds": map[string]string{
			"Tmdb":    intToStr(m.TMDbID),
			"Bangumi": intToStr(m.BangumiID),
		},
		"UserData": map[string]any{
			"PlaybackPositionTicks": posMs * 10_000,
			"PlayCount":             0,
			"IsFavorite":            fav,
			"Played":                played,
			"PlayedPercentage":      pct,
		},
		"MediaSources": e.mediaSourcesForItem(ctx, m, true, false),
	}
	if premiered, ok := embyPremiereDate(m.ReleaseDate); ok {
		item["PremiereDate"] = premiered
	}
	return item
}
