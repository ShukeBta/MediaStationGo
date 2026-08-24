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
	return deduplicate(out)
}
