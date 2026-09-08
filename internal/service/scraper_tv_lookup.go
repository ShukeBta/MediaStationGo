package service

import (
	"context"
	"fmt"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type tvLookupKey struct {
	Query string
	Year  int
}
type tvLookupResult struct {
	Matches []*Match
	Err     error
}

// Cache provider results, not a chosen identity: every file must independently
// agree with the candidate. Failures are cached only within this run.
func (s *ScraperService) lookupTVStrict(ctx context.Context, media *model.Media, query string, year int, options ScrapeOptions) (*Match, error) {
	key := tvLookupKey{query, year}
	result, found := options.tvLookup[key]
	if !found {
		if s.tmdb == nil || !s.tmdb.Enabled() {
			return nil, fmt.Errorf("TMDB 不可用，剧集匹配未完成")
		}
		result.Matches, result.Err = s.tmdb.SearchTVCandidates(ctx, query, year)
		if options.tvLookup != nil {
			options.tvLookup[key] = result
		}
	}
	if result.Err != nil {
		return nil, fmt.Errorf("TMDB 剧集搜索请求失败，保留原状态: %w", result.Err)
	}
	var selected *Match
	for _, candidate := range result.Matches {
		if !episodePathTitleTrusted(media.Path, candidate) || (year > 0 && candidate.Year != year) {
			continue
		}
		if selected != nil && selected.TMDbID != candidate.TMDbID {
			return nil, fmt.Errorf("同名剧集存在多个候选，请确认版本或首播年份")
		}
		selected = candidate
	}
	return cloneManualScrapeMatch(selected), nil
}
