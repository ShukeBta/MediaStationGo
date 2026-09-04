package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// movieLibraryHasEpisodicContent reports whether a movie-like library needs
// logical grouping for episodic rows or multipart videos.
func (e *EmbyService) movieLibraryHasEpisodicContent(ctx context.Context, libraryID string) (bool, error) {
	clause, args := embyLikelyEpisodicPathSQL()
	if clause == "" {
		clause = "1 = 0"
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID)).
		Where("COALESCE(part_group_key, '') <> '' OR ((season_num > 0 OR episode_num > 0) AND ("+clause+"))", args...)
	var count int64
	if err := q.Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (e *EmbyService) libraryHasMultipartContent(ctx context.Context, libraryID string) bool {
	if strings.TrimSpace(libraryID) == "" {
		return false
	}
	var count int64
	err := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ? AND COALESCE(part_group_key, '') <> ''", e.mergedLibraryIDs(ctx, libraryID)).
		Limit(1).Count(&count).Error
	return err == nil && count > 0
}

// movieLibraryItems 处理电影类型库的常规浏览,返回「真正的电影(Movie)」与
// 「库内剧集结构内容聚成的 Series 卡片」的合并列表(按 DateCreated 倒序分页)。
// 与 mediaItems 的区别: 后者会把剧集结构行当散装 Episode 漏出;这里改为聚合成
// Series,从根本上消除「电影库里整部剧被拆成单集」的现象。
func (e *EmbyService) movieLibraryItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	cacheKey := e.embyItemsCacheKey("movie-library", p)
	var cached embyItemsCacheValue
	if e.cache != nil && e.cache.GetJSON(ctx, cacheKey, &cached) {
		return map[string]any{"Items": cached.Items, "TotalRecordCount": int(cached.TotalRecordCount), "StartIndex": cached.StartIndex}, nil
	}
	if e.cache != nil {
		call, owner := e.beginEmbyReadCacheFill(cacheKey)
		if !owner {
			if err := waitEmbyReadCacheFill(ctx, call); err != nil {
				return nil, err
			}
			if e.cache.GetJSON(ctx, cacheKey, &cached) {
				return map[string]any{"Items": cached.Items, "TotalRecordCount": int(cached.TotalRecordCount), "StartIndex": cached.StartIndex}, nil
			}
		} else {
			defer e.finishEmbyReadCacheFill(cacheKey, call)
		}
	}

	libIDs := e.mergedLibraryIDs(ctx, p.ParentID)
	hasMultipart := e.libraryHasMultipartContent(ctx, p.ParentID)
	queryOrder := embyMovieLibraryOrderSQL(p)
	includeSeries := len(p.IncludeItemTypes) == 0 || containsItemType(p.IncludeItemTypes, "Series") || hasMultipart
	includeMovies := len(p.IncludeItemTypes) == 0 || containsItemType(p.IncludeItemTypes, "Movie")
	apply := func(q *gorm.DB) *gorm.DB {
		q = e.applyUserMediaVisibility(ctx, q, p.UserID)
		q = q.Where("library_id IN ?", libIDs)
		q = applyEmbyMediaSearch(q, p)
		if containsEmbyFilter(p.Filters, "IsFavorite") {
			if strings.TrimSpace(p.UserID) == "" {
				return nil
			}
			q = q.Joins("JOIN favorites ON favorites.media_id = media.id AND favorites.user_id = ? AND favorites.deleted_at IS NULL", p.UserID)
		}
		return q
	}

	// 剧集结构内容 -> Series 卡片。
	clause, args := embyLikelyEpisodicPathSQL()
	var episodicRows []model.Media
	if includeSeries && clause != "" {
		epQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
		if epQ == nil {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
		}
		epQ = epQ.Where("(season_num > 0 OR episode_num > 0) AND ("+clause+")", args...).
			Order(queryOrder).Limit(embySeriesGroupingLimit)
		if err := epQ.Find(&episodicRows).Error; err != nil {
			return nil, err
		}
		episodicRows = e.filterMediaRowsByEmbyGenres(episodicRows, p)
	}
	seriesGroups, err := e.seriesGroupsFromMedia(ctx, episodicRows)
	if err != nil {
		return nil, err
	}
	if includeSeries && hasMultipart {
		multipartQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
		if multipartQ == nil {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
		}
		var multipartRows []model.Media
		if err := multipartQ.Where("COALESCE(part_group_key, '') <> ''").
			Order(queryOrder + ", media.part_group_key ASC, media.part_index ASC").
			Limit(embySeriesGroupingLimit).Find(&multipartRows).Error; err != nil {
			return nil, err
		}
		multipartRows = e.filterMediaRowsByEmbyGenres(multipartRows, p)
		seriesGroups = append(seriesGroups, e.multipartSeriesGroupsFromMedia(multipartRows)...)
	}

	// 真正的电影 -> Movie 项(剔除剧集结构行)。
	var movieRows []model.Media
	if includeMovies {
		movieQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
		if movieQ == nil {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
		}
		movieQ = filterLikelyEpisodicPathsFromMovieQuery(movieQ).
			Where("COALESCE(part_group_key, '') = ''").
			Order(queryOrder).Limit(embySeriesGroupingLimit)
		if err := movieQ.Find(&movieRows).Error; err != nil {
			return nil, err
		}
		movieRows = e.filterMediaRowsByEmbyGenres(movieRows, p)
	}
	movieRows = e.collapseMediaVersionRows(ctx, movieRows)
	entries := e.movieLibraryEntries(ctx, seriesGroups, movieRows)
	descending := !strings.EqualFold(firstCSVValue(p.SortOrder), "Ascending")
	switch primarySupportedEmbySort(p.SortBy, false) {
	case "datecreated":
		sort.SliceStable(entries, func(i, j int) bool {
			left := entries[i].createdAt
			right := entries[j].createdAt
			if left.Equal(right) {
				// The merged Series/Movie list is paged in memory. Its secondary
				// key must be the same public Id clients receive, not a display name.
				if descending {
					return entries[i].id > entries[j].id
				}
				return entries[i].id < entries[j].id
			}
			if descending {
				return left.After(right)
			}
			return left.Before(right)
		})
	case "sortname", "name":
		sort.SliceStable(entries, func(i, j int) bool {
			left := entries[i].name
			right := entries[j].name
			if descending {
				return left > right
			}
			return left < right
		})
	default:
		sort.SliceStable(entries, func(i, j int) bool {
			if descending {
				return entries[i].sortAt.After(entries[j].sortAt)
			}
			return entries[i].sortAt.Before(entries[j].sortAt)
		})
	}
	total := len(entries)
	paged := pageSlice(entries, p.StartIndex, p.Limit)
	items, err := e.movieLibraryPayloads(ctx, p, paged)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}
	if e.cache != nil {
		e.cache.SetJSON(ctx, cacheKey, embyItemsCacheValue{Items: items, TotalRecordCount: int64(total), StartIndex: p.StartIndex}, e.embyMediaCacheTTL())
	}
	return out, nil
}

type embyMovieLibraryEntry struct {
	id        string
	sortAt    time.Time
	createdAt time.Time
	name      string
	series    *embySeriesGroup
	media     *model.Media
}

func (e *EmbyService) movieLibraryEntries(ctx context.Context, seriesGroups []embySeriesGroup, movieRows []model.Media) []embyMovieLibraryEntry {
	entries := make([]embyMovieLibraryEntry, 0, len(seriesGroups)+len(movieRows))
	for index := range seriesGroups {
		group := &seriesGroups[index]
		entries = append(entries, embyMovieLibraryEntry{
			id:        group.ID,
			sortAt:    embySeriesReleaseSortTime(*group),
			createdAt: group.CreatedAt,
			name:      strings.ToLower(strings.TrimSpace(group.Name)),
			series:    group,
		})
	}

	adultLibraries := map[string]bool{}
	knownAdultLibraries := map[string]bool{}
	for index := range movieRows {
		media := &movieRows[index]
		adult := media.NSFW
		libraryID := strings.TrimSpace(media.LibraryID)
		if !adult && libraryID != "" {
			if knownAdultLibraries[libraryID] {
				adult = adultLibraries[libraryID]
			} else if e != nil && e.repo != nil && e.repo.Library != nil {
				library, err := e.repo.Library.FindByID(ctx, libraryID)
				adult = err == nil && library != nil && LibraryIsAdult(*library)
				knownAdultLibraries[libraryID] = true
				adultLibraries[libraryID] = adult
			}
		}
		entries = append(entries, embyMovieLibraryEntry{
			id:        media.ID,
			sortAt:    embyMoviePayloadReleaseSortTime(*media),
			createdAt: media.CreatedAt,
			name:      strings.ToLower(strings.TrimSpace(adultDisplayNameForMedia(media, media.Title, adult))),
			media:     media,
		})
	}
	return entries
}

func (e *EmbyService) movieLibraryPayloads(ctx context.Context, p ItemsParams, entries []embyMovieLibraryEntry) ([]map[string]any, error) {
	movieRows := make([]model.Media, 0, len(entries))
	for _, entry := range entries {
		if entry.media != nil {
			movieRows = append(movieRows, *entry.media)
		}
	}
	movieItems, err := e.payloadsForMediaRows(ctx, movieRows, p.UserID, !p.OmitMediaSources, false)
	if err != nil {
		return nil, err
	}
	if len(movieItems) != len(movieRows) {
		return nil, fmt.Errorf("movie library payload count mismatch: got %d, want %d", len(movieItems), len(movieRows))
	}

	items := make([]map[string]any, 0, len(entries))
	movieIndex := 0
	for _, entry := range entries {
		if entry.series != nil {
			items = append(items, e.seriesPayload(*entry.series))
			continue
		}
		if entry.media != nil {
			items = append(items, movieItems[movieIndex])
			movieIndex++
		}
	}
	return items, nil
}

func embyMovieLibraryOrderSQL(p ItemsParams) string {
	descending := !strings.EqualFold(firstCSVValue(p.SortOrder), "Ascending")
	direction := " ASC"
	if descending {
		direction = " DESC"
	}
	switch primarySupportedEmbySort(p.SortBy, false) {
	case "datecreated":
		return "media.created_at" + direction + ", media.id" + direction
	case "sortname", "name":
		return "media.title" + direction
	case "communityrating":
		return "media.rating" + direction
	default:
		return mediaReleaseOrderSQL(descending)
	}
}

func embyMoviePayloadReleaseSortTime(media model.Media) time.Time {
	if premiered, ok := embyPremiereDate(media.ReleaseDate); ok {
		return premiered
	}
	if media.Year > 0 {
		return time.Date(media.Year, time.December, 31, 0, 0, 0, 0, time.UTC)
	}
	return media.CreatedAt
}

func (e *EmbyService) libraryIsEpisodic(ctx context.Context, libraryID string) (bool, error) {
	if strings.TrimSpace(libraryID) == "" {
		return false, nil
	}
	if lib, loaded := embyLibraryFromSnapshot(ctx, libraryID); loaded {
		if lib != nil {
			return embyLibraryTypeIsEpisodic(lib.Type), nil
		}
	} else if lib, err := e.repo.Library.FindByID(ctx, libraryID); err != nil {
		return false, err
	} else if lib != nil {
		return embyLibraryTypeIsEpisodic(lib.Type), nil
	}
	var count int64
	err := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ? AND (season_num > 0 OR episode_num > 0)", e.mergedLibraryIDs(ctx, libraryID)).
		Count(&count).Error
	return count > 0, err
}

func (e *EmbyService) mediaBelongsToEpisodicLibrary(ctx context.Context, m *model.Media) bool {
	if e == nil || m == nil || strings.TrimSpace(m.LibraryID) == "" {
		return false
	}
	if lib, loaded := embyLibraryFromSnapshot(ctx, m.LibraryID); loaded {
		return lib != nil && embyLibraryTypeIsEpisodic(lib.Type)
	}
	lib, err := e.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil || lib == nil {
		return false
	}
	return embyLibraryTypeIsEpisodic(lib.Type)
}

func (e *EmbyService) mediaShouldBeEpisode(ctx context.Context, m *model.Media) bool {
	if m == nil || (m.SeasonNum <= 0 && m.EpisodeNum <= 0) {
		return false
	}
	if strings.TrimSpace(m.PartGroupKey) != "" {
		return true
	}
	if e.mediaBelongsToEpisodicLibrary(ctx, m) {
		return true
	}
	return embyMediaPathLooksEpisodic(m.Path)
}

func embyLibraryTypeIsEpisodic(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "tv", "anime", "variety":
		return true
	default:
		return false
	}
}

func (e *EmbyService) filterMovieItems(ctx context.Context, q *gorm.DB) *gorm.DB {
	episodicIDs := e.episodicLibraryIDs(ctx)
	if len(episodicIDs) == 0 {
		return filterLikelyEpisodicPathsFromMovieQuery(q).Where("COALESCE(media.part_group_key, '') = ''")
	}
	q = q.Where("((media.season_num = 0 AND media.episode_num = 0) OR media.library_id NOT IN ?) AND COALESCE(media.part_group_key, '') = ''", episodicIDs)
	return filterLikelyEpisodicPathsFromMovieQuery(q)
}

func (e *EmbyService) filterEpisodeItems(ctx context.Context, q *gorm.DB) *gorm.DB {
	episodicIDs := e.episodicLibraryIDs(ctx)
	if len(episodicIDs) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where("media.library_id IN ? AND (media.season_num > 0 OR media.episode_num > 0)", episodicIDs)
}

func (e *EmbyService) episodicLibraryIDs(ctx context.Context) []string {
	if e == nil || e.repo == nil || e.repo.DB == nil {
		return nil
	}
	var ids []string
	if err := e.repo.DB.WithContext(ctx).Model(&model.Library{}).
		Where("LOWER(type) IN ?", []string{"tv", "anime", "variety"}).
		Pluck("id", &ids).Error; err != nil {
		return nil
	}
	return ids
}

func filterLikelyEpisodicPathsFromMovieQuery(q *gorm.DB) *gorm.DB {
	clause, args := embyLikelyEpisodicPathSQL()
	if clause == "" {
		return q
	}
	return q.Where("NOT ((media.season_num > 0 OR media.episode_num > 0) AND ("+clause+"))", args...)
}

func embyLikelyEpisodicPathSQL() (string, []any) {
	patterns := []string{
		"%/season %/%", "%/season.%/%", "%/season-%/%", "%/season_%/%",
		"%/s0%/%", "%/s1%/%", "%/s2%/%", "%/s3%/%", "%/s4%/%", "%/s5%/%", "%/s6%/%", "%/s7%/%", "%/s8%/%", "%/s9%/%",
		"%/special/%", "%/specials/%", "%/sp/%", "%/ova/%", "%/oad/%", "%/extra/%", "%/extras/%",
		"%/电视剧/%", "%/剧集/%", "%/连续剧/%", "%/短剧/%", "%/国产剧/%", "%/国剧/%", "%/大陆剧/%", "%/华语剧/%", "%/国产电视剧/%", "%/大陆电视剧/%", "%/华语电视剧/%", "%/欧美剧/%", "%/欧美电视剧/%", "%/美剧/%", "%/英剧/%", "%/日韩剧/%", "%/日韩电视剧/%", "%/日剧/%", "%/韩剧/%", "%/港剧/%", "%/台剧/%", "%/港台剧/%", "%/泰剧/%",
		"%/日番/%", "%/国漫/%", "%/番剧/%", "%/动漫/%", "%/特别篇/%", "%/特別篇/%", "%/番外/%", "%/特典/%",
	}
	clauses := make([]string, 0, len(patterns)*2)
	args := make([]any, 0, len(patterns)*2)
	for _, pattern := range patterns {
		clauses = append(clauses, "LOWER(media.path) LIKE ?")
		args = append(args, pattern)
		if strings.Contains(pattern, "/") {
			clauses = append(clauses, "LOWER(media.path) LIKE ?")
			args = append(args, strings.ReplaceAll(pattern, "/", `\`))
		}
	}
	return strings.Join(clauses, " OR "), args
}

func embyMediaPathLooksEpisodic(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(path), "\\", "/"))
	if normalized == "" {
		return false
	}
	for _, marker := range []string{
		"/season ", "/season.", "/season-", "/season_", "/special/", "/specials/", "/sp/", "/ova/", "/oad/", "/extra/", "/extras/",
		"/电视剧/", "/剧集/", "/连续剧/", "/短剧/", "/国产剧/", "/国剧/", "/大陆剧/", "/华语剧/", "/国产电视剧/", "/大陆电视剧/", "/华语电视剧/", "/欧美剧/", "/欧美电视剧/", "/美剧/", "/英剧/", "/日韩剧/", "/日韩电视剧/", "/日剧/", "/韩剧/", "/港剧/", "/台剧/", "/港台剧/", "/泰剧/",
		"/日番/", "/国漫/", "/番剧/", "/动漫/", "/特别篇/", "/特別篇/", "/番外/", "/特典/",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	for _, marker := range []string{"/s0", "/s1", "/s2", "/s3", "/s4", "/s5", "/s6", "/s7", "/s8", "/s9"} {
		if idx := strings.Index(normalized, marker); idx >= 0 {
			after := idx + len(marker)
			if after < len(normalized) && normalized[after] >= '0' && normalized[after] <= '9' {
				slash := after + 1
				if slash < len(normalized) && normalized[slash] == '/' {
					return true
				}
			}
		}
	}
	return false
}
