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
	selected, err := selectStrictTVCandidate(media, year, result.Matches)
	if err != nil || selected != nil || !queryNeedsEnglishTMDbFallback(query) || tvCandidatesContainQueryTitle(query, result.Matches) {
		return cloneManualScrapeMatch(selected), err
	}
	// 上游:中文结果里找不到可信候选且查询含拉丁字母时(英文发行名),补一次
	// en-US 搜索,合并同 ID 候选后再选,命中后本地化为中文标题。
	altKey := tvLookupKey{query + "\x00en-US", year}
	alternate, found := options.tvLookup[altKey]
	if !found {
		alternate.Matches, alternate.Err = s.tmdb.searchTVCandidates(ctx, query, year, "en-US")
		if options.tvLookup != nil {
			options.tvLookup[altKey] = alternate
		}
	}
	if alternate.Err != nil || len(alternate.Matches) == 0 {
		return nil, nil
	}
	selected, err = selectStrictTVCandidate(media, year, mergeTMDbLanguageCandidates(result.Matches, alternate.Matches))
	if err != nil || selected == nil {
		return nil, err
	}
	return cloneManualScrapeMatch(s.localizeAutomaticTMDbMatch(ctx, "tv", query, selected)), nil
}

func selectStrictTVCandidate(media *model.Media, year int, candidates []*Match) (*Match, error) {
	var selected *Match
	for _, candidate := range candidates {
		if !episodePathTitleTrusted(media.Path, candidate) || (year > 0 && candidate.Year != year) {
			continue
		}
		if selected != nil && selected.TMDbID != candidate.TMDbID {
			return nil, fmt.Errorf("同名剧集存在多个候选，请确认版本或首播年份")
		}
		selected = candidate
	}
	return selected, nil
}

// tvCandidatesContainQueryTitle 表示中文结果里已有与查询同名的候选(只是
// 路径校验未通过),此时换语言重搜不会带来新身份,跳过回退请求。
func tvCandidatesContainQueryTitle(query string, candidates []*Match) bool {
	queryKey := metadataTrustKey(query)
	if queryKey == "" {
		return false
	}
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		titles := append([]string{candidate.Title, candidate.OriginalName}, candidate.Aliases...)
		for _, title := range titles {
			if metadataTrustKey(title) == queryKey {
				return true
			}
		}
	}
	return false
}
