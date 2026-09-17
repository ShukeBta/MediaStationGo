package service

import (
	"strconv"
	"strings"
)

var tmdbMovieGenreNames = map[int]string{
	12:    "冒险",
	14:    "奇幻",
	16:    "动画",
	18:    "剧情",
	27:    "恐怖",
	28:    "动作",
	35:    "喜剧",
	36:    "历史",
	37:    "西部",
	53:    "惊悚",
	80:    "犯罪",
	99:    "纪录片",
	878:   "科幻",
	9648:  "悬疑",
	10402: "音乐",
	10749: "爱情",
	10751: "家庭",
	10752: "战争",
	10770: "电视电影",
}

var tmdbTVGenreNames = map[int]string{
	16:    "动画",
	18:    "剧情",
	35:    "喜剧",
	37:    "西部",
	80:    "犯罪",
	99:    "纪录片",
	9648:  "悬疑",
	10751: "家庭",
	10759: "动作冒险",
	10762: "儿童",
	10763: "新闻",
	10764: "真人秀",
	10765: "科幻奇幻",
	10766: "肥皂剧",
	10767: "脱口秀",
	10768: "战争政治",
}

var standardGenreNames = map[string]string{
	"action":             "动作",
	"action & adventure": "动作冒险",
	"adventure":          "冒险",
	"animation":          "动画",
	"comedy":             "喜剧",
	"crime":              "犯罪",
	"documentary":        "纪录片",
	"drama":              "剧情",
	"family":             "家庭",
	"fantasy":            "奇幻",
	"history":            "历史",
	"horror":             "恐怖",
	"kids":               "儿童",
	"music":              "音乐",
	"mystery":            "悬疑",
	"news":               "新闻",
	"reality":            "真人秀",
	"romance":            "爱情",
	"sci-fi":             "科幻",
	"sci-fi & fantasy":   "科幻奇幻",
	"science fiction":    "科幻",
	"science-fiction":    "科幻",
	"soap":               "肥皂剧",
	"talk":               "脱口秀",
	"thriller":           "惊悚",
	"tv movie":           "电视电影",
	"war":                "战争",
	"war & politics":     "战争政治",
	"western":            "西部",
}

// deduplicate removes duplicates from a string slice.
func deduplicate(s []string) []string {
	if len(s) == 0 {
		return s
	}
	seen := make(map[string]bool, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func tmdbGenreNames(mediaType string, ids []int) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := tmdbGenreName(mediaType, id); ok {
			out = append(out, name)
		}
	}
	return deduplicate(out)
}

func tmdbGenreName(mediaType string, id int) (string, bool) {
	switch normalizeOrganizeMediaType(mediaType) {
	case "tv", "anime", "variety":
		name, ok := tmdbTVGenreNames[id]
		return name, ok
	case "movie":
		name, ok := tmdbMovieGenreNames[id]
		return name, ok
	default:
		if name, ok := tmdbMovieGenreNames[id]; ok {
			return name, true
		}
		name, ok := tmdbTVGenreNames[id]
		return name, ok
	}
}

func normalizeTMDbGenreValues(mediaType string, values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		id, err := strconv.Atoi(value)
		if err == nil {
			out = append(out, tmdbGenreNames(mediaType, []int{id})...)
			continue
		}
		out = append(out, value)
	}
	return normalizeStandardGenreValues(out)
}

// normalizeStandardGenreValues 把各元数据源常见的英文标准类型统一为中文。
// 未知值原样保留，避免把来源自定义但确属 genre 的分类静默丢弃。
func normalizeStandardGenreValues(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = cleanXMLText(value)
		if value == "" {
			continue
		}
		if localized, ok := standardGenreNames[strings.ToLower(value)]; ok {
			value = localized
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
