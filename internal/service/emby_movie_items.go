package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// movieLibraryHasEpisodicContent 报告电影类型库里是否混入了「剧集结构」内容
// (有季集号且路径形如剧集,例如 .../国产剧/某剧/Season 01/某剧 - S01E01.mkv)。
// 用于决定是否需要走 movieLibraryItems 把这些内容聚成 Series 卡片。普通电影库
// 没有这类行时返回 false,继续走常规 mediaItems。
func (e *EmbyService) movieLibraryHasEpisodicContent(ctx context.Context, libraryID string) (bool, error) {
	clause, args := embyLikelyEpisodicPathSQL()
	if clause == "" {
		return false, nil
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID)).
		Where("(season_num > 0 OR episode_num > 0) AND ("+clause+")", args...)
	var count int64
	if err := q.Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// movieLibraryItems 处理电影类型库的常规浏览,返回「真正的电影(Movie)」与
// 「库内剧集结构内容聚成的 Series 卡片」的合并列表(按 DateCreated 倒序分页)。
// 与 mediaItems 的区别: 后者会把剧集结构行当散装 Episode 漏出;这里改为聚合成
// Series,从根本上消除「电影库里整部剧被拆成单集」的现象。
func (e *EmbyService) movieLibraryItems(ctx context.Context, p ItemsParams) (map[string]any, error) {
	libIDs := e.mergedLibraryIDs(ctx, p.ParentID)
	apply := func(q *gorm.DB) *gorm.DB {
		q = e.applyUserMediaVisibility(ctx, q, p.UserID)
		q = q.Where("library_id IN ?", libIDs)
		if p.SearchTerm != "" {
			q = q.Where("title LIKE ? OR original_name LIKE ?", "%"+p.SearchTerm+"%", "%"+p.SearchTerm+"%")
		}
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
	if clause != "" {
		epQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
		if epQ == nil {
			return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
		}
		epQ = epQ.Where("(season_num > 0 OR episode_num > 0) AND ("+clause+")", args...).
			Order(mediaReleaseOrderSQL(true)).Limit(embySeriesGroupingLimit)
		if err := epQ.Find(&episodicRows).Error; err != nil {
			return nil, err
		}
	}
	seriesGroups := e.seriesGroupsFromMedia(episodicRows)

	// 真正的电影 -> Movie 项(剔除剧集结构行)。
	movieQ := apply(e.repo.DB.WithContext(ctx).Model(&model.Media{}))
	if movieQ == nil {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
	}
	movieQ = filterLikelyEpisodicPathsFromMovieQuery(movieQ).
		Order(mediaReleaseOrderSQL(true)).Limit(embySeriesGroupingLimit)
	var movieRows []model.Media
	if err := movieQ.Find(&movieRows).Error; err != nil {
		return nil, err
	}
	movieItems, err := e.payloadsForMedia(ctx, movieRows, p.UserID)
	if err != nil {
		return nil, err
	}

	// 合并: Series 卡片 + Movie 项, 统一按首播/上映日期倒序。
	type entry struct {
		sortAt  time.Time
		payload map[string]any
	}
	entries := make([]entry, 0, len(seriesGroups)+len(movieItems))
	for _, g := range seriesGroups {
		entries = append(entries, entry{sortAt: embySeriesReleaseSortTime(g), payload: e.seriesPayload(g)})
	}
	for _, item := range movieItems {
		entries = append(entries, entry{sortAt: embyPayloadReleaseSortTime(item), payload: item})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].sortAt.After(entries[j].sortAt)
	})
	total := len(entries)
	paged := pageSlice(entries, p.StartIndex, p.Limit)
	items := make([]map[string]any, 0, len(paged))
	for _, en := range paged {
		items = append(items, en.payload)
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

// embyPayloadCreatedAt 从 item payload 里取 DateCreated(time.Time),用于合并排序。
func embyPayloadCreatedAt(item map[string]any) time.Time {
	if item == nil {
		return time.Time{}
	}
	if v, ok := item["DateCreated"].(time.Time); ok {
		return v
	}
	return time.Time{}
}

func embyPayloadReleaseSortTime(item map[string]any) time.Time {
	if item == nil {
		return time.Time{}
	}
	if v, ok := item["PremiereDate"].(time.Time); ok {
		return v
	}
	if year, ok := item["ProductionYear"].(int); ok && year > 0 {
		return time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	}
	return embyPayloadCreatedAt(item)
}

func (e *EmbyService) libraryIsEpisodic(ctx context.Context, libraryID string) (bool, error) {
	if strings.TrimSpace(libraryID) == "" {
		return false, nil
	}
	if lib, err := e.repo.Library.FindByID(ctx, libraryID); err != nil {
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
		return filterLikelyEpisodicPathsFromMovieQuery(q)
	}
	q = q.Where("(media.season_num = 0 AND media.episode_num = 0) OR media.library_id NOT IN ?", episodicIDs)
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
