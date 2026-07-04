package service

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var episodicPathRE = regexp.MustCompile(`(?i)[\\/](?:电视剧|剧集|连续剧|短剧|国产剧|国剧|大陆剧|华语剧|国产电视剧|大陆电视剧|华语电视剧|欧美剧|欧美电视剧|美剧|英剧|日韩剧|日韩电视剧|日剧|韩剧|港剧|台剧|港台剧|泰剧|综艺|纪录片|儿童|动漫|番剧|国漫|日番|韩漫|美漫|欧美动漫|欧美动画|其他动漫|tv|series|shows?|season[\s._-]*\d|s\d{1,2}(?:[\s._-]|[\\/])|special[\s._-]*episodes?|specials?|sp|ovas?|oads?|extras?|bonus(?:es)?|omake|特别篇|特別篇|番外篇?|特典|外传|外傳|总集篇|總集篇)[\\/]`)

func mediaSeriesKey(media model.Media) string {
	return compactSeriesKey(mediaSeriesRawKey(media))
}

func mediaSeriesRawKey(media model.Media) string {
	fromPath := seriesTitleFromMediaPath(media.Path)
	if media.SeasonNum > 0 || media.EpisodeNum > 0 || episodicPathRE.MatchString(media.Path+" "+media.DisplayLibraryPath+" "+media.LibraryPath) {
		if fromPath != "" {
			return seriesFingerprint("library-path", mediaTargetLibraryID(media), fromPath)
		}
		if idKey := seriesExternalIDKeyFromPath(media.Path); idKey != "" {
			return seriesFingerprint("library-path-id", mediaTargetLibraryID(media), idKey)
		}
		if media.TMDbID > 0 {
			return fmt.Sprintf("tmdb:%d", media.TMDbID)
		}
		if media.BangumiID > 0 {
			return fmt.Sprintf("bgm:%d", media.BangumiID)
		}
		if strings.TrimSpace(media.DoubanID) != "" {
			return "douban:" + strings.TrimSpace(media.DoubanID)
		}
		if strings.TrimSpace(media.TheTVDBID) != "" {
			return "thetvdb:" + strings.TrimSpace(media.TheTVDBID)
		}
		if strings.TrimSpace(media.SeriesID) != "" {
			return "series:" + strings.TrimSpace(media.SeriesID)
		}
		return seriesFingerprint("library-title", mediaTargetLibraryID(media), normalizeSeriesTitle(seriesDisplayTitle(media)))
	}
	if strings.TrimSpace(media.SeriesID) != "" {
		return "series:" + strings.TrimSpace(media.SeriesID)
	}
	if media.TMDbID > 0 {
		return fmt.Sprintf("tmdb:%d", media.TMDbID)
	}
	if media.BangumiID > 0 {
		return fmt.Sprintf("bgm:%d", media.BangumiID)
	}
	if fromPath != "" {
		return seriesFingerprint("library-path", media.LibraryID, fromPath)
	}
	return seriesFingerprint("library-title", media.LibraryID, normalizeSeriesTitle(media.Title))
}

func seriesFingerprint(parts ...string) string {
	return strings.Join(parts, "\x1f")
}

func compactSeriesKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var hash uint32 = 2166136261
	for _, b := range []byte(raw) {
		hash ^= uint32(b)
		hash *= 16777619
	}
	return fmt.Sprintf("series:%08x", hash)
}

var (
	seriesYearRE        = regexp.MustCompile(`\s*\((?:19|20)\d{2}\)\s*`)
	seriesIDRE          = regexp.MustCompile(`(?i)\s*\[(?:tmdb|tmdbid)[=-]\d+\]\s*`)
	seriesBraceRE       = regexp.MustCompile(`(?i)\s*\{(?:tmdb|tmdbid|douban|bangumi|bgm|thetvdb|tvdb)[\s:=#-]*[a-z0-9_-]+\}\s*`)
	seriesSpacerRE      = regexp.MustCompile(`[\s._-]+`)
	seriesSeasonDirRE   = regexp.MustCompile(`(?i)^(?:s\d{1,2}|season[\s._-]*\d{1,2}|第\s*[0-9一二三四五六七八九十百零两]+\s*季|special[\s._-]*episodes?|specials?|sp|ovas?|oads?|extras?|bonus(?:es)?|omake|特别篇|特別篇|番外篇?|特典|外传|外傳|总集篇|總集篇)$`)
	seriesSpecialCodeRE = regexp.MustCompile(`(?i)\s*[\[(（【]?\s*(?:s0+\s*e?\s*\d+|season\s*0+(?:\s*episode)?\s*\d*|special(?:\s*episode)?s?\s*\d*|sp\s*\d*|ovas?\s*\d*|oads?\s*\d*|extras?\s*\d*|bonus(?:es)?\s*\d*|omake\s*\d*)\s*[\])）】]?$`)
	seriesSpecialCJKRE  = regexp.MustCompile(`(?i)\s*[\[(（【]?\s*(?:特别篇|特別篇|番外篇?|特典|外传|外傳|总集篇|總集篇)(?:\s*第?\s*[0-9一二三四五六七八九十百零两]+(?:[集话話期])?)?\s*[\])）】]?$`)
)

func normalizeSeriesTitle(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = seriesYearRE.ReplaceAllString(value, " ")
	value = seriesIDRE.ReplaceAllString(value, " ")
	value = seriesBraceRE.ReplaceAllString(value, " ")
	value = seriesSpacerRE.ReplaceAllString(value, " ")
	return strings.TrimSpace(value)
}

func normalizeSeriesPathTitle(value string) string {
	title, _ := CleanQuery(value)
	if title == "" {
		title = normalizeSeriesTitle(value)
	} else {
		title = normalizeSeriesTitle(title)
	}
	stripped := stripSeriesSpecialSuffix(title)
	if stripped != "" {
		return stripped
	}
	return title
}

func stripSeriesSpecialSuffix(title string) string {
	for _, re := range []*regexp.Regexp{seriesSpecialCodeRE, seriesSpecialCJKRE} {
		stripped := strings.TrimSpace(re.ReplaceAllString(title, ""))
		if stripped != "" && stripped != title {
			return stripped
		}
	}
	return title
}

func seriesTitleFromMediaPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return ""
	}
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
	if len(parts) < 2 {
		return ""
	}
	dirIndex := len(parts) - 2
	if last := parts[len(parts)-1]; !seriesPathPartLooksLikeFile(last) && !seriesSeasonDirRE.MatchString(filepath.Base(last)) {
		dirIndex = len(parts) - 1
	}
	for dirIndex >= 0 && seriesSeasonDirRE.MatchString(filepath.Base(parts[dirIndex])) {
		dirIndex--
	}
	if dirIndex < 0 {
		return ""
	}
	title := normalizeSeriesPathTitle(parts[dirIndex])
	if unsafeAutomaticEpisodeQuery(title) {
		return ""
	}
	return title
}

func seriesPathPartLooksLikeFile(part string) bool {
	ext := strings.ToLower(filepath.Ext(part))
	if _, ok := videoExtensions[ext]; ok {
		return true
	}
	switch ext {
	case ".iso", ".nfo", ".srt", ".ass", ".ssa", ".vtt", ".sub", ".idx", ".jpg", ".jpeg", ".png", ".webp":
		return true
	default:
		return false
	}
}

func seriesDisplayTitle(media model.Media) string {
	if fromPath := seriesTitleFromMediaPath(media.Path); fromPath != "" {
		return fromPath
	}
	if media.Title != "" {
		return media.Title
	}
	if media.OriginalName != "" {
		return media.OriginalName
	}
	return "未命名节目"
}

func seriesExternalIDKeyFromPath(path string) string {
	_, hints := pathHintMetadata(path, true)
	switch {
	case hints.TMDbID > 0:
		return fmt.Sprintf("tmdb:%d", hints.TMDbID)
	case hints.BangumiID > 0:
		return fmt.Sprintf("bgm:%d", hints.BangumiID)
	case strings.TrimSpace(hints.DoubanID) != "":
		return "douban:" + strings.TrimSpace(hints.DoubanID)
	case strings.TrimSpace(hints.TheTVDBID) != "":
		return "thetvdb:" + strings.TrimSpace(hints.TheTVDBID)
	default:
		return ""
	}
}

func mediaTargetLibraryID(media model.Media) string {
	if strings.TrimSpace(media.DisplayLibraryID) != "" {
		return media.DisplayLibraryID
	}
	return media.LibraryID
}
