package service

import (
	"fmt"
	"regexp"
	"strings"
)

func embyHasMediaFilter(p ItemsParams) bool {
	return embyHasMediaSearch(p) || hasEmbyGenreFilter(p)
}

func containsItemType(types []string, want string) bool {
	for _, t := range types {
		if strings.EqualFold(strings.TrimSpace(t), want) {
			return true
		}
	}
	return false
}

func normalizeEmbyGlobalSearchParams(p ItemsParams) ItemsParams {
	if !embyHasMediaSearch(p) || strings.TrimSpace(p.ParentID) != "" {
		return p
	}
	p.IncludeItemTypes = nil
	return p
}

func containsSupportedEmbyItemType(types []string) bool {
	for _, itemType := range types {
		switch strings.ToLower(strings.TrimSpace(itemType)) {
		case "movie", "series", "season", "episode", "video", "folder", "collectionfolder":
			return true
		}
	}
	return false
}

func containsOnlyFolderItemTypes(types []string) bool {
	if len(types) == 0 {
		return false
	}
	for _, itemType := range types {
		switch strings.ToLower(strings.TrimSpace(itemType)) {
		case "folder", "collectionfolder":
		default:
			return false
		}
	}
	return true
}

func emptyItemsEnvelope(startIndex int) map[string]any {
	return map[string]any{
		"Items":            []map[string]any{},
		"TotalRecordCount": int64(0),
		"StartIndex":       startIndex,
	}
}

func containsEmbyFilter(filters []string, want string) bool {
	for _, filter := range filters {
		if strings.EqualFold(strings.TrimSpace(filter), want) {
			return true
		}
	}
	return false
}

func firstCSVValue(value string) string {
	if i := strings.Index(value, ","); i >= 0 {
		value = value[:i]
	}
	return strings.TrimSpace(value)
}

func primarySupportedEmbySort(sortBy string, resumeFilter bool) string {
	for _, part := range strings.Split(sortBy, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		switch key {
		case "sortname", "name", "premieredate", "productionyear", "datecreated", "communityrating":
			return key
		case "dateplayed":
			if resumeFilter {
				return key
			}
		}
	}
	if resumeFilter {
		return "dateplayed"
	}
	return strings.ToLower(strings.TrimSpace(firstCSVValue(sortBy)))
}

func embyDatePlayedOrder(desc bool) string {
	if desc {
		return "resume.watched_at DESC, media.updated_at DESC, media.id DESC"
	}
	return "resume.watched_at ASC, media.updated_at ASC, media.id ASC"
}

func pageSlice[T any](items []T, start, limit int) []T {
	if start < 0 {
		start = 0
	}
	if limit <= 0 {
		limit = len(items)
	}
	if start >= len(items) {
		return []T{}
	}
	end := start + limit
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}

func emptyUserData() map[string]any {
	return map[string]any{
		"PlaybackPositionTicks": 0,
		"PlayCount":             0,
		"IsFavorite":            false,
		"Played":                false,
		"PlayedPercentage":      0,
	}
}

// embyEpisodeNumberedNameRE 匹配“第 N 集”形态的单集名（避免二次加前缀）。
var embyEpisodeNumberedNameRE = regexp.MustCompile(`^第\s*\d+\s*集`)

// embyDecorateEpisodeRowTitle 给“最近观看/NextUp”这类横排卡片里的单集
// 标题补上集数（爆米花等客户端渲染卡片时不显示 IndexNumber 徽标，
// 只拼 SeriesName/SeasonName/Name，没有集数就看不出上次看到第几集）。
func embyDecorateEpisodeRowTitle(item map[string]any) {
	if item == nil || !strings.EqualFold(strings.TrimSpace(embyPayloadString(item, "Type", "type")), "Episode") {
		return
	}
	episodeNum, ok := item["IndexNumber"].(int)
	if !ok || episodeNum <= 0 {
		return
	}
	name := strings.TrimSpace(embyPayloadString(item, "Name", "name"))
	if name == "" || embyEpisodeNumberedNameRE.MatchString(name) {
		return
	}
	if _, exists := item["EpisodeTitle"]; !exists {
		item["EpisodeTitle"] = name
	}
	item["Name"] = fmt.Sprintf("第%d集 %s", episodeNum, name)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
