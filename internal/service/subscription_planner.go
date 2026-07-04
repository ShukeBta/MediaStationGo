// Package service — subscription planning and release candidate selection.
package service

import "github.com/ShukeBta/MediaStationGo/internal/model"

type siteSearchCandidate struct {
	Item     SearchResult
	Download string
	GUID     string
	Season   int
	Episode  int
	Episodes []int
	Pack     bool
	Score    int
}

type siteSearchSelectionStats struct {
	Total                    int
	QueryMismatch            int
	QueryMismatchExamples    []string
	RelaxedQueryMatch        int
	RuleMismatch             int
	MissingDownload          int
	Seen                     int
	Prepared                 int
	Selected                 int
	LocalAlreadySatisfied    bool
	LocalSeriesPackPresent   bool
	SeriesComplete           bool
	ExistingEpisodeSkipped   int
	NotMissingEpisodeSkipped int
	NoEpisodeSkipped         int
	PackFallbackAvailable    bool
	PackFallbackUsed         bool
}

// SubscriptionPlanner owns release selection decisions for subscriptions:
// rule matching, candidate scoring, and filtering against known availability.
type SubscriptionPlanner struct{}

func selectSiteSearchCandidates(results []SearchResult, sub *model.Subscription, seenSet map[string]struct{}, availability ...LocalAvailability) []siteSearchCandidate {
	return SubscriptionPlanner{}.SelectSiteSearchCandidates(results, sub, seenSet, availability...)
}

func (SubscriptionPlanner) SelectSiteSearchCandidates(results []SearchResult, sub *model.Subscription, seenSet map[string]struct{}, availability ...LocalAvailability) []siteSearchCandidate {
	if sub == nil {
		return nil
	}
	if seenSet == nil {
		seenSet = map[string]struct{}{}
	}
	local := LocalAvailability{}
	if len(availability) > 0 {
		local = availability[0]
	}
	candidates, _ := selectSiteSearchCandidatesWithStats(results, sub, seenSet, local)
	return candidates
}

func selectSiteSearchCandidatesWithAvailability(results []SearchResult, sub *model.Subscription, seenSet map[string]struct{}, local LocalAvailability) []siteSearchCandidate {
	candidates, _ := selectSiteSearchCandidatesWithStats(results, sub, seenSet, local)
	return candidates
}

func selectSiteSearchCandidatesWithStats(results []SearchResult, sub *model.Subscription, seenSet map[string]struct{}, local LocalAvailability) ([]siteSearchCandidate, siteSearchSelectionStats) {
	stats := siteSearchSelectionStats{Total: len(results)}
	if sub == nil {
		return nil, stats
	}
	if seenSet == nil {
		seenSet = map[string]struct{}{}
	}
	candidates := collectSiteSearchCandidates(results, sub, seenSet, false, &stats)
	if len(candidates) == 0 && shouldRelaxSiteSearchQueryMatch(sub, local) && stats.QueryMismatch > 0 {
		relaxedStats := siteSearchSelectionStats{Total: len(results)}
		candidates = collectSiteSearchCandidates(results, sub, seenSet, true, &relaxedStats)
		stats.RuleMismatch = relaxedStats.RuleMismatch
		stats.MissingDownload = relaxedStats.MissingDownload
		stats.Seen = relaxedStats.Seen
		stats.Prepared = relaxedStats.Prepared
		stats.RelaxedQueryMatch = relaxedStats.RelaxedQueryMatch
	}
	selected := selectPreparedSubscriptionCandidatesWithStats(candidates, sub, local, &stats)
	return selected, stats
}
