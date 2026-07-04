// Package service — episode parser for TV series.
//
// Detects season + episode numbers from filenames. Recognised patterns:
//
//	S01E02        / s1e2
//	1x02          / 01x02
//	EP02 / E02
//	第2集         / 第02集
//
// For bare episode markers such as "EP02", the parser also looks at parent
// folders like "Season 02" / "S02" / "第2季" before falling back to season 1.
//
// When neither a season nor an episode marker is present, returns (0, 0).
package service

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	patSEnE          = regexp.MustCompile(`(?i)s(\d{1,2})e(\d{1,3})`)
	patSEnERange     = regexp.MustCompile(`(?i)s(\d{1,2})e(\d{1,3})\s*[-~–—]\s*(?:s(\d{1,2}))?e?(\d{1,3})(?:[^0-9]|$)`)
	patDanglingSE    = regexp.MustCompile(`(?i)(?:^|[\s._-])s\d{1,2}e(?:[\s._-]|$)`)
	patNxE           = regexp.MustCompile(`(\d{1,2})x(\d{1,3})`)
	patEP            = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:e|ep)\.?\s*(\d{1,3})(?:[^0-9]|$)`)
	patCN            = regexp.MustCompile(`第\s*([0-9一二三四五六七八九十百零两]+)\s*[集话話期]`)
	patCNRange       = regexp.MustCompile(`第\s*([0-9一二三四五六七八九十百零两]+)\s*[-~–—]\s*([0-9一二三四五六七八九十百零两]+)\s*[集话話期]`)
	patDashEpisode   = regexp.MustCompile(`[\s._-][-–—]\s*(\d{1,3})(?:\s*(?:v\d+)?)?(?:\s*[\[\(._-]|$)`)
	patSeasonFolder  = regexp.MustCompile(`(?i)(?:^|[^a-z])(?:s|season)\.?\s*(\d{1,2})(?:[^0-9]|$)|第\s*([0-9一二三四五六七八九十百零两]+)\s*季`)
	patSeasonOnly    = regexp.MustCompile(`(?i)(?:^|[\s._-])(?:s|season)\.?\s*\d{1,2}(?:[\s._-]|$)`)
	patBareEpisode   = regexp.MustCompile(`^(?:第\s*)?0?(\d{1,3})(?:\s*(?:v\d+)?)?$`)
	patSpecialSeason = regexp.MustCompile(`(?i)^(?:s0+|season[\s._-]*0+|special[\s._-]*episodes?|specials?|sp|ovas?|oads?|extras?|bonus(?:es)?|omake|番外篇?|特别篇|特別篇|特典|外传|外傳|总集篇|總集篇)$`)
	// patCNSeason 匹配中文季/部标记，支持阿拉伯数字与中文数字（如「第二季」「第2部」）。
	patCNSeason = regexp.MustCompile(`第\s*[0-9一二三四五六七八九十百零两]+\s*[季部]`)
)

// ParseEpisode tries to extract (season, episode) from an arbitrary filename.
// Returns (0, 0) when nothing recognisable is found.
func ParseEpisode(path string) (season, episode int) {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	if m := patSEnE.FindStringSubmatch(name); len(m) == 3 {
		season = mustAtoi(m[1])
		episode = mustAtoi(m[2])
		return
	}
	if m := patNxE.FindStringSubmatch(name); len(m) == 3 {
		season = mustAtoi(m[1])
		episode = mustAtoi(m[2])
		return
	}
	if m := patEP.FindStringSubmatch(name); len(m) >= 2 {
		var found bool
		season, found = seasonFromParents(path)
		if !found {
			season = 1
		}
		episode = mustAtoi(m[1])
		return
	}
	if m := patCN.FindStringSubmatch(name); len(m) >= 2 {
		var found bool
		season, found = seasonFromParents(path)
		if !found {
			season = 1
		}
		episode = mustAtoi(m[1])
		return
	}
	if m := patDashEpisode.FindStringSubmatch(name); len(m) >= 2 {
		var found bool
		season, found = seasonFromParents(path)
		if !found {
			season = 1
		}
		episode = mustAtoi(m[1])
		return
	}
	if parentSeason, found := seasonFromParents(path); found {
		if m := patBareEpisode.FindStringSubmatch(strings.TrimSpace(name)); len(m) >= 2 {
			season = parentSeason
			episode = mustAtoi(m[1])
			return
		}
	}
	return 0, 0
}

type episodeRef struct {
	Season  int
	Episode int
}

func episodeRefsFromTitle(path string) []episodeRef {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if refs := parseSEpisodeRange(name); len(refs) > 0 {
		return refs
	}
	if refs := parseCNEpisodeRange(name, path); len(refs) > 0 {
		return refs
	}
	season, episode := ParseEpisode(path)
	if episode <= 0 {
		return nil
	}
	if season <= 0 {
		season = 1
	}
	return []episodeRef{{Season: season, Episode: episode}}
}

func parseSEpisodeRange(name string) []episodeRef {
	m := patSEnERange.FindStringSubmatch(name)
	if len(m) < 5 {
		return nil
	}
	season := mustAtoi(m[1])
	endSeason := season
	if m[3] != "" {
		endSeason = mustAtoi(m[3])
	}
	if season <= 0 || endSeason != season {
		return nil
	}
	start := mustAtoi(m[2])
	end := mustAtoi(m[4])
	return buildEpisodeRefRange(season, start, end)
}

func parseCNEpisodeRange(name, path string) []episodeRef {
	m := patCNRange.FindStringSubmatch(name)
	if len(m) < 3 {
		return nil
	}
	season, found := seasonFromParents(path)
	if !found {
		season = 1
	}
	return buildEpisodeRefRange(season, mustAtoi(m[1]), mustAtoi(m[2]))
}

func buildEpisodeRefRange(season, start, end int) []episodeRef {
	if season <= 0 {
		season = 1
	}
	if start <= 0 || end <= 0 || end < start || end-start > 200 {
		return nil
	}
	refs := make([]episodeRef, 0, end-start+1)
	for episode := start; episode <= end; episode++ {
		refs = append(refs, episodeRef{Season: season, Episode: episode})
	}
	return refs
}

func seasonFromParents(path string) (int, bool) {
	dir := filepath.Dir(path)
	for i := 0; i < 4; i++ {
		base := filepath.Base(dir)
		if base == "." || base == string(filepath.Separator) {
			return 0, false
		}
		if season, ok := seasonFromDir(base); ok {
			return season, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return 0, false
		}
		dir = parent
	}
	return 0, false
}

func seasonFromDir(name string) (int, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, false
	}
	if patSpecialSeason.MatchString(name) {
		return 0, true
	}
	if m := patSeasonFolder.FindStringSubmatch(name); len(m) >= 3 {
		for _, group := range m[1:] {
			if group != "" {
				return mustAtoi(group), true
			}
		}
	}
	return 0, false
}

func mustAtoi(s string) int {
	s = strings.TrimSpace(s)
	if v, err := strconv.Atoi(s); err == nil {
		return v
	}
	v := parseChineseNumber(s)
	if v > 0 {
		return v
	}
	return v
}

func parseChineseNumber(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	digit := func(r rune) (int, bool) {
		switch r {
		case '零', '〇':
			return 0, true
		case '一':
			return 1, true
		case '二', '两':
			return 2, true
		case '三':
			return 3, true
		case '四':
			return 4, true
		case '五':
			return 5, true
		case '六':
			return 6, true
		case '七':
			return 7, true
		case '八':
			return 8, true
		case '九':
			return 9, true
		default:
			return 0, false
		}
	}
	if utf8.RuneCountInString(s) == 1 {
		if v, ok := digit([]rune(s)[0]); ok {
			return v
		}
	}
	total := 0
	current := 0
	for _, r := range s {
		switch r {
		case '百':
			if current == 0 {
				current = 1
			}
			total += current * 100
			current = 0
		case '十':
			if current == 0 {
				current = 1
			}
			total += current * 10
			current = 0
		default:
			v, ok := digit(r)
			if !ok {
				return 0
			}
			current = v
		}
	}
	return total + current
}
