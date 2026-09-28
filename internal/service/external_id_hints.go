package service

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type mediaExternalIDHints struct {
	TMDbID    int
	BangumiID int
	DoubanID  string
	TheTVDBID string
}

var externalIDHintPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"tmdb", regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:tmdb|tmdbid)[\s_:=#-]*(\d{2,})`)},
	{"bangumi", regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:bangumi|bgm)[\s_:=#-]*(\d{2,})`)},
	{"douban", regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:douban|db)[\s_:=#-]*(\d{2,})`)},
	{"thetvdb", regexp.MustCompile(`(?i)(?:^|[^a-z0-9])(?:thetvdb|tvdb)[\s_:=#-]*(\d{2,})`)},
}

var (
	imdbIDRE          = regexp.MustCompile(`(?i)tt\d{2,20}`)
	doubanSubjectIDRE = regexp.MustCompile(`(?i)(?:douban\.com/(?:movie/)?subject/|/subject/)(\d{2,32})`)
	digitGroupRE      = regexp.MustCompile(`\d{2,32}`)
	tmdbIDRE          = regexp.MustCompile(`(?i)(?:themoviedb\.org/(?:movie|tv)/|(?:^|[^a-z0-9])(?:tmdb|tmdbid)[\s_:=#/-]*)(\d{2,})`)
)

func NormalizeIMDBID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if match := imdbIDRE.FindString(raw); match != "" {
		return strings.ToLower(match)
	}
	if digits := digitGroupRE.FindString(raw); digits != "" {
		return "tt" + digits
	}
	return ""
}

func NormalizeDoubanID(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if m := doubanSubjectIDRE.FindStringSubmatch(raw); len(m) >= 2 {
		return m[1]
	}
	if digitGroupRE.MatchString(raw) {
		return digitGroupRE.FindString(raw)
	}
	return ""
}

func NormalizeTMDbID(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	if m := tmdbIDRE.FindStringSubmatch(raw); len(m) >= 2 {
		return mustAtoi(m[1])
	}
	if digits := digitGroupRE.FindString(raw); digits != "" {
		return mustAtoi(digits)
	}
	return 0
}

func externalIDHintsFromText(raw string) mediaExternalIDHints {
	var hints mediaExternalIDHints
	for _, item := range externalIDHintPatterns {
		m := item.re.FindStringSubmatch(raw)
		if len(m) < 2 {
			continue
		}
		value := strings.TrimSpace(m[1])
		switch item.name {
		case "tmdb":
			hints.TMDbID = NormalizeTMDbID(value)
		case "bangumi":
			hints.BangumiID = mustAtoi(value)
		case "douban":
			hints.DoubanID = NormalizeDoubanID(value)
		case "thetvdb":
			hints.TheTVDBID = value
		}
	}
	return hints
}

func (h mediaExternalIDHints) useful() bool {
	return h.TMDbID > 0 || h.BangumiID > 0 || strings.TrimSpace(h.DoubanID) != "" || strings.TrimSpace(h.TheTVDBID) != ""
}

func (h mediaExternalIDHints) applyToLocalMetadata(meta *LocalMetadata) *LocalMetadata {
	if !h.useful() {
		return meta
	}
	if meta == nil {
		meta = &LocalMetadata{}
	}
	if h.TMDbID > 0 && meta.TMDbID <= 0 {
		meta.TMDbID = h.TMDbID
	}
	if h.BangumiID > 0 && meta.BangumiID <= 0 {
		meta.BangumiID = h.BangumiID
	}
	if doubanID := NormalizeDoubanID(h.DoubanID); doubanID != "" && meta.DoubanID == "" {
		meta.DoubanID = doubanID
	}
	if strings.TrimSpace(h.TheTVDBID) != "" && meta.TheTVDBID == "" {
		meta.TheTVDBID = strings.TrimSpace(h.TheTVDBID)
	}
	meta.PathHint = true
	return meta
}

func pathHintMetadata(raw string, seriesLike bool) (*LocalMetadata, mediaExternalIDHints) {
	source := pathHintSourceText(raw, seriesLike)
	hints := externalIDHintsFromText(source)
	title, year := "", 0
	if seriesLike {
		title, year = CleanQuery(source)
	} else {
		title, year = cloudSeriesTitleFromMediaPath(source)
		if title == "" {
			title, year = CleanQuery(source)
		}
	}
	if !hints.useful() && title == "" && year <= 0 {
		return nil, hints
	}
	meta := (&mediaExternalIDHints{
		TMDbID:    hints.TMDbID,
		BangumiID: hints.BangumiID,
		DoubanID:  hints.DoubanID,
		TheTVDBID: hints.TheTVDBID,
	}).applyToLocalMetadata(&LocalMetadata{PathHint: true})
	if title != "" {
		meta.Title = title
	}
	if year > 0 {
		meta.Year = year
	}
	return meta, hints
}

func pathHintSourceText(raw string, seriesLike bool) string {
	raw = strings.TrimSpace(raw)
	if !seriesLike || raw == "" {
		return raw
	}
	base := pathBaseSlash(raw)
	ext := strings.ToLower(filepath.Ext(base))
	if _, ok := videoExtensions[ext]; !ok {
		return raw
	}
	if showDir := showDirFromEpisodePath(raw); showDir != "" {
		return showDir
	}
	return raw
}
