package service

import (
	"context"
	"strings"

	"go.uber.org/zap"
)

func (s *ScraperService) lookupAutomaticTMDb(ctx context.Context, kind, query string, year int) *Match {
	var (
		candidates []*Match
		err        error
		mediaLabel string
	)
	if isTVMetadataKind(kind) {
		mediaLabel = "tv"
		candidates, err = s.tmdb.SearchTVCandidates(ctx, query, year)
	} else {
		mediaLabel = "movie"
		candidates, err = s.tmdb.SearchMovieCandidates(ctx, query, year)
	}
	if err != nil {
		s.log.Debug("tmdb "+mediaLabel+" search failed", zap.String("query", query), zap.Error(err))
		return nil
	}
	if match := bestAutomaticMetadataMatch(query, year, kind, candidates); match != nil {
		return s.localizeAutomaticTMDbMatch(ctx, kind, query, match)
	}
	if !queryNeedsEnglishTMDbFallback(query) {
		return nil
	}

	var alternate []*Match
	if isTVMetadataKind(kind) {
		alternate, err = s.tmdb.searchTVCandidates(ctx, query, year, "en-US")
	} else {
		alternate, err = s.tmdb.searchMovieCandidates(ctx, query, year, "en-US")
	}
	if err != nil {
		s.log.Debug("tmdb "+mediaLabel+" alternate-language search failed", zap.String("query", query), zap.Error(err))
		return nil
	}
	match := bestAutomaticMetadataMatch(query, year, kind, mergeTMDbLanguageCandidates(candidates, alternate))
	return s.localizeAutomaticTMDbMatch(ctx, kind, query, match)
}

func (s *ScraperService) localizeAutomaticTMDbMatch(ctx context.Context, kind, query string, match *Match) *Match {
	if s == nil || s.tmdb == nil || match == nil || match.TMDbID <= 0 || !metadataTitleNeedsChineseLocalization(match) {
		return match
	}
	var (
		localized *Match
		err       error
	)
	if isTVMetadataKind(kind) {
		localized, err = s.tmdb.GetTVMatch(ctx, match.TMDbID)
	} else {
		localized, err = s.tmdb.GetMovieMatch(ctx, match.TMDbID)
	}
	if err != nil || localized == nil {
		if err != nil && s.log != nil {
			s.log.Debug("tmdb localized title lookup failed",
				zap.String("query", query),
				zap.Int("tmdb_id", match.TMDbID),
				zap.Error(err))
		}
		return match
	}
	mergeAutomaticTMDbLocalizedMatch(localized, match)
	localized.SearchKeyword = query
	localized.Aliases = appendMetadataAliases(localized.Aliases,
		match.Title, match.OriginalName)
	localized.Aliases = appendMetadataAliases(localized.Aliases, match.Aliases...)
	return localized
}

func mergeAutomaticTMDbLocalizedMatch(localized, search *Match) {
	if localized == nil || search == nil {
		return
	}
	if strings.TrimSpace(localized.OriginalName) == "" {
		localized.OriginalName = strings.TrimSpace(firstNonEmpty(search.OriginalName, search.Title))
	}
	localized.Overview = firstNonEmpty(localized.Overview, search.Overview)
	localized.PosterURL = firstNonEmpty(localized.PosterURL, search.PosterURL)
	localized.BackdropURL = firstNonEmpty(localized.BackdropURL, search.BackdropURL)
	localized.MediaType = firstNonEmpty(localized.MediaType, search.MediaType)
	if localized.Year <= 0 {
		localized.Year = search.Year
	}
	if localized.ReleaseDate == "" {
		localized.ReleaseDate = search.ReleaseDate
	}
	if localized.Rating <= 0 {
		localized.Rating = search.Rating
	}
	if len(localized.Languages) == 0 {
		localized.Languages = append([]string(nil), search.Languages...)
	}
	if len(localized.Countries) == 0 {
		localized.Countries = append([]string(nil), search.Countries...)
	}
	if len(localized.Genres) == 0 {
		localized.Genres = append([]string(nil), search.Genres...)
	}
}

func bestAutomaticMetadataMatch(query string, year int, expectedType string, candidates []*Match) *Match {
	bestScore := -1
	var best *Match
	for index, candidate := range candidates {
		if candidate == nil || !metadataMatchCompatibleWithType(expectedType, candidate) {
			continue
		}
		if !organizeMetadataMatchTrusted(query, year, candidate) {
			continue
		}
		score := automaticMetadataMatchScore(query, year, candidate) - index
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}
	return best
}

func automaticMetadataMatchScore(query string, year int, match *Match) int {
	queryKey := metadataTrustKey(query)
	score := 0
	titles := make([]string, 0, 2+len(match.Aliases))
	titles = append(titles, match.Title, match.OriginalName)
	titles = append(titles, match.Aliases...)
	for _, title := range titles {
		titleKey := metadataTrustKey(title)
		switch {
		case titleKey != "" && titleKey == queryKey:
			if score < 1000 {
				score = 1000
			}
		case metadataTrustTokenOverlap(queryKey, titleKey):
			if score < 700 {
				score = 700
			}
		}
	}
	if year > 0 && match.Year > 0 {
		diff := year - match.Year
		if diff < 0 {
			diff = -diff
		}
		switch diff {
		case 0:
			score += 200
		case 1:
			score += 100
		}
	}
	if metadataMatchHasExternalID(match) {
		score += 20
	}
	if strings.TrimSpace(match.PosterURL) != "" {
		score += 5
	}
	return score
}

func metadataMatchCompatibleWithType(expectedType string, match *Match) bool {
	if match == nil {
		return false
	}
	expectedType = normalizeOrganizeMediaType(expectedType)
	matchType := normalizeOrganizeMediaType(match.MediaType)
	if expectedType == "" || matchType == "" {
		return true
	}
	switch expectedType {
	case "tv", "anime", "variety":
		return matchType == "tv" || matchType == "anime" || matchType == "variety"
	case "movie", "adult":
		return matchType == "movie" || matchType == "adult"
	default:
		return expectedType == matchType
	}
}

func queryNeedsEnglishTMDbFallback(query string) bool {
	for _, r := range query {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			return true
		}
	}
	return false
}

func mergeTMDbLanguageCandidates(primary, alternate []*Match) []*Match {
	if len(primary) == 0 {
		return alternate
	}
	byID := make(map[int]*Match, len(primary))
	out := append([]*Match(nil), primary...)
	for _, candidate := range primary {
		if candidate != nil && candidate.TMDbID > 0 {
			byID[candidate.TMDbID] = candidate
		}
	}
	for _, candidate := range alternate {
		if candidate == nil {
			continue
		}
		if localized := byID[candidate.TMDbID]; localized != nil {
			localized.Aliases = appendMetadataAliases(localized.Aliases,
				candidate.Title, candidate.OriginalName)
			localized.Aliases = appendMetadataAliases(localized.Aliases, candidate.Aliases...)
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func appendMetadataAliases(existing []string, values ...string) []string {
	seen := make(map[string]struct{}, len(existing)+len(values))
	out := make([]string, 0, len(existing)+len(values))
	add := func(value string) {
		value = strings.TrimSpace(value)
		key := metadataTrustKey(value)
		if value == "" || key == "" {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	for _, value := range existing {
		add(value)
	}
	for _, value := range values {
		add(value)
	}
	return out
}
