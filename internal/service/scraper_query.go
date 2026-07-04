package service

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var (
	episodeOnlyQueryRE       = regexp.MustCompile(`(?i)^\s*(?:e(?:p(?:isode)?)?\s*\d{1,3}|episode\s*\d{1,3}|第\s*[0-9一二三四五六七八九十百零两]+\s*[集期话話](?:\s*[上下])?)\s*$`)
	episodeTitleQueryRE      = regexp.MustCompile(`^\s*第\s*[0-9一二三四五六七八九十百零两]+\s*[集期话話](?:\s*[上下])?\s*[:：].+`)
	genericEpisodeWordsRE    = regexp.MustCompile(`^\s*第\s*[集期话話]\s*$`)
	episodeReleaseTitleTagRE = regexp.MustCompile(`(?i)(?:^|[\s._-])s\d{1,2}e\d{1,3}(?:[\s._-]|$)`)
)

func scrapeQueryCandidates(m *model.Media, lib *model.Library) []string {
	return scrapeQueryCandidatesWithNormalizer(m, lib, func(raw string) (string, int) {
		return CleanQuery(raw)
	})
}

func scrapeQueryCandidatesWithRecognition(ctx context.Context, repo *repository.Container, m *model.Media, lib *model.Library) []string {
	return scrapeQueryCandidatesWithNormalizer(m, lib, func(raw string) (string, int) {
		return CleanQueryWithRecognition(ctx, repo, raw)
	})
}

func scrapeQueryCandidatesWithNormalizer(m *model.Media, lib *model.Library, clean func(string) (string, int)) []string {
	seen := map[string]struct{}{}
	var out []string
	add := func(raw string) {
		cleaned, _ := clean(raw)
		if cleaned == "" {
			cleaned = strings.TrimSpace(raw)
		}
		for _, candidate := range titleCandidates(cleaned) {
			if unsafeAutomaticEpisodeQuery(candidate) {
				continue
			}
			key := strings.ToLower(candidate)
			if _, ok := seen[key]; ok || candidate == "" {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, candidate)
		}
	}
	episodic := mediaIsEpisodic(m, lib)
	if lib != nil && episodic {
		add(seriesFolderTitle(m.Path, lib.Path))
	}
	if lib != nil {
		add(mediaFolderTitle(m.Path, lib.Path))
	}
	add(m.Title)
	add(m.Path)
	if len(out) == 0 {
		base := pathBaseSlash(m.Path)
		out = append(out, strings.TrimSuffix(base, filepath.Ext(base)))
	}
	return out
}

func titleCandidates(title string) []string {
	title = strings.Join(strings.Fields(strings.TrimSpace(title)), " ")
	if title == "" {
		return nil
	}
	out := make([]string, 0, 2)
	if cjk := cjkTitleOnly(title); cjk != "" {
		out = append(out, cjk)
		if cjk != title {
			return out
		}
	}
	out = append(out, title)
	return out
}

func cjkTitleOnly(title string) string {
	parts := make([]string, 0, 4)
	for _, field := range strings.Fields(title) {
		if containsCJK(field) {
			parts = append(parts, field)
		}
	}
	return strings.Join(parts, " ")
}

func containsCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= '\u3400' && r <= '\u4dbf':
			return true
		case r >= '\u4e00' && r <= '\u9fff':
			return true
		case r >= '\uf900' && r <= '\ufaff':
			return true
		}
	}
	return false
}

func mediaIsEpisodic(m *model.Media, lib *model.Library) bool {
	if m != nil && (m.SeasonNum > 0 || m.EpisodeNum > 0) {
		return true
	}
	if m != nil {
		season, episode := ParseEpisode(m.Path)
		if season > 0 || episode > 0 {
			return true
		}
	}
	return librarySupportsSeasons(lib)
}

func librarySupportsSeasons(lib *model.Library) bool {
	if lib == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(lib.Type)) {
	case "tv", "anime", "variety", "show", "shows":
		return true
	default:
		return false
	}
}

func unsafeAutomaticEpisodeQuery(query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	if episodeOnlyQueryRE.MatchString(query) || genericEpisodeWordsRE.MatchString(query) {
		return true
	}
	if episodeTitleQueryRE.MatchString(query) {
		return true
	}
	_, episode := ParseEpisode(query)
	if episode > 0 && !looksLikeSeriesReleaseTitle(query) {
		return true
	}
	return false
}

func looksLikeSeriesReleaseTitle(query string) bool {
	cleaned, _ := CleanQuery(query)
	return strings.TrimSpace(cleaned) != "" && episodeReleaseTitleTagRE.MatchString(query)
}
