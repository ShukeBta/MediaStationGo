package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type manualTMDbCandidate struct {
	MediaType string
	Match     *Match
}

func (s *ScraperService) manualTMDbCandidates(ctx context.Context, query string, year int, mediaType string) []manualTMDbCandidate {
	if s.tmdb == nil || !s.tmdb.Enabled() {
		return nil
	}
	if id, ok := parseProviderIDInt(query, "tmdb"); ok {
		out := make([]manualTMDbCandidate, 0, 2)
		for _, typ := range manualTMDbIDSearchTypes(mediaType) {
			if match := s.manualTMDbMatchByIDForType(ctx, id, typ); match != nil {
				out = append(out, manualTMDbCandidate{MediaType: typ, Match: match})
			}
		}
		return out
	}
	if providerIDHintMismatched(query, "tmdb") {
		return nil
	}
	out := make([]manualTMDbCandidate, 0, 4)
	for _, typ := range manualTMDbSearchTypes(mediaType) {
		switch typ {
		case "movie":
			if matches, err := s.tmdb.SearchMovieCandidates(ctx, query, year); err == nil {
				for _, match := range matches {
					out = append(out, manualTMDbCandidate{MediaType: "movie", Match: match})
				}
			}
		case "tv":
			if matches, err := s.tmdb.SearchTVCandidates(ctx, query, year); err == nil {
				for _, match := range matches {
					out = append(out, manualTMDbCandidate{MediaType: "tv", Match: match})
				}
			}
		}
	}
	return out
}

func (s *ScraperService) manualTMDbMatches(ctx context.Context, query string, year int, mediaType string) []*Match {
	candidates := s.manualTMDbCandidates(ctx, query, year, mediaType)
	out := make([]*Match, 0, len(candidates))
	for _, candidate := range candidates {
		out = append(out, candidate.Match)
	}
	return out
}

func manualTMDbIDSearchTypes(mediaType string) []string {
	switch normalizeMediaType(mediaType, "", "") {
	case "tv", "anime", "variety":
		return []string{"tv", "movie"}
	case "movie", "adult":
		return []string{"movie", "tv"}
	default:
		return []string{"movie", "tv"}
	}
}

func manualTMDbSearchTypes(mediaType string) []string {
	if strings.TrimSpace(mediaType) == "" {
		return []string{"movie", "tv"}
	}
	switch normalizeMediaType(mediaType, "", "") {
	case "tv", "anime", "variety":
		return []string{"tv", "movie"}
	case "movie", "adult":
		return []string{"movie"}
	default:
		return []string{"movie", "tv"}
	}
}

func (s *ScraperService) manualAdultMatches(ctx context.Context, media *model.Media, query string) []*Match {
	candidates := []string{query}
	if media != nil {
		candidates = append(candidates, media.Path, media.OriginalName, media.Title)
	}
	out := make([]*Match, 0, 1)
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		code := normalizeAdultCode(candidate)
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		if match := s.manualAdultMatch(ctx, code); match != nil {
			out = append(out, match)
		}
	}
	return out
}

func (s *ScraperService) manualAdultMatch(ctx context.Context, code string) *Match {
	if s.adult == nil || !s.adult.Enabled() {
		return nil
	}
	match, err := s.adult.Search(ctx, code)
	if err != nil || match == nil {
		return nil
	}
	return match
}

func (s *ScraperService) manualTMDbMatchByID(ctx context.Context, id int, mediaType string) *Match {
	if s.tmdb == nil || !s.tmdb.Enabled() || id <= 0 {
		return nil
	}
	if mediaType == "tv" || mediaType == "anime" || mediaType == "variety" {
		if match, err := s.tmdb.GetTVMatch(ctx, id); err == nil && match != nil {
			return match
		}
	}
	if match, err := s.tmdb.GetMovieMatch(ctx, id); err == nil && match != nil {
		return match
	}
	if match, err := s.tmdb.GetTVMatch(ctx, id); err == nil && match != nil {
		return match
	}
	return nil
}

func (s *ScraperService) manualTMDbMatchByIDForType(ctx context.Context, id int, mediaType string) *Match {
	if s.tmdb == nil || !s.tmdb.Enabled() || id <= 0 {
		return nil
	}
	switch normalizeMediaType(mediaType, "", "") {
	case "tv", "anime", "variety":
		if match, err := s.tmdb.GetTVMatch(ctx, id); err == nil && match != nil {
			return match
		}
	case "movie", "adult":
		if match, err := s.tmdb.GetMovieMatch(ctx, id); err == nil && match != nil {
			return match
		}
	}
	return nil
}

func (s *ScraperService) manualDoubanMatch(ctx context.Context, query string) *Match {
	if s.douban == nil || !s.douban.Enabled() {
		return nil
	}
	if id, ok := parseProviderIDString(query, "douban"); ok {
		if match, err := s.douban.GetMatchByID(ctx, id); err == nil && match != nil {
			return match
		}
	}
	if providerIDHintMismatched(query, "douban") {
		return nil
	}
	match, err := s.douban.SearchMatch(ctx, query)
	if err != nil {
		return nil
	}
	return match
}

func (s *ScraperService) manualBangumiMatch(ctx context.Context, query string) *Match {
	if s.bangumi == nil || !s.bangumi.Enabled() {
		return nil
	}
	if id, ok := parseProviderIDInt(query, "bangumi"); ok {
		if match, err := s.bangumi.GetSubject(ctx, id); err == nil && match != nil {
			return match
		}
	}
	if providerIDHintMismatched(query, "bangumi") {
		return nil
	}
	match, err := s.bangumi.Search(ctx, query)
	if err != nil {
		return nil
	}
	return match
}

func (s *ScraperService) manualTheTVDBMatch(ctx context.Context, query string) *Match {
	if s.thetvdb == nil || !s.thetvdb.Enabled() {
		return nil
	}
	if id, ok := parseProviderIDString(query, "thetvdb"); ok {
		if match, err := s.thetvdb.GetSeriesMatchByID(ctx, id); err == nil && match != nil {
			return match
		}
	}
	if providerIDHintMismatched(query, "thetvdb") {
		return nil
	}
	if id, ok := parseBarePositiveIDString(normalizeTheTVDBSeriesID(query)); ok {
		if match, err := s.thetvdb.GetSeriesMatchByID(ctx, id); err == nil && match != nil {
			return match
		}
	}
	match, err := s.thetvdb.SearchSeries(ctx, query)
	if err != nil {
		return nil
	}
	return match
}
