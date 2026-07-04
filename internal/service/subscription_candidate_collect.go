package service

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var looseSubscriptionEpisodeRE = regexp.MustCompile(`(?i)(?:^|[\s._\-\[\(])0?(\d{1,3})(?:v\d+)?(?:$|[\s._\-\]\)])`)

func collectSiteSearchCandidates(results []SearchResult, sub *model.Subscription, seenSet map[string]struct{}, allowQueryMismatch bool, stats *siteSearchSelectionStats) []siteSearchCandidate {
	candidates := make([]siteSearchCandidate, 0, len(results))
	for _, item := range results {
		matchText := subscriptionSearchResultText(item)
		if !subscriptionSearchResultMatchesQuery(sub, item) {
			if allowQueryMismatch {
				stats.RelaxedQueryMatch++
			} else {
				stats.QueryMismatch++
				stats.QueryMismatchExamples = appendLimitedStrings(stats.QueryMismatchExamples, matchText, 5)
				continue
			}
		}
		if !matchesSubscriptionRules(sub, matchText) || !matchesSubscriptionTorrentRules(sub, item) {
			stats.RuleMismatch++
			continue
		}
		download := strings.TrimSpace(item.DownloadURL)
		if download == "" {
			download = strings.TrimSpace(item.TorrentURL)
		}
		if download == "" {
			stats.MissingDownload++
			continue
		}
		guid := stableSiteSearchGUID(item, download)
		if _, ok := seenSet[guid]; ok {
			stats.Seen++
			continue
		}
		refs := subscriptionCandidateEpisodeRefs(sub, matchText)
		season, episode := ParseEpisode(matchText)
		if episode <= 0 && len(refs) > 0 {
			season = refs[0].Season
			episode = refs[0].Episode
		}
		episodes := episodeNumbersFromRefs(refs, season)
		score := subscriptionCandidateScore(sub, item)
		stats.Prepared++
		candidates = append(candidates, siteSearchCandidate{
			Item:     item,
			Download: download,
			GUID:     guid,
			Season:   season,
			Episode:  episode,
			Episodes: episodes,
			Pack:     isSeriesPackTitle(item.Title) || len(episodes) > 1,
			Score:    score,
		})
	}
	return candidates
}

func appendLimitedStrings(values []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" || limit <= 0 || len(values) >= limit {
		return values
	}
	return append(values, value)
}

func shouldRelaxSiteSearchQueryMatch(sub *model.Subscription, local LocalAvailability) bool {
	if sub == nil {
		return false
	}
	mediaType := normalizeMediaType(sub.MediaType, sub.Name+" "+sub.Filter, "")
	if !isSubscriptionSeriesType(mediaType) {
		return false
	}
	if local.LocalMediaCount == 0 && len(local.ExistingEpisodeKeys) == 0 {
		return false
	}
	return local.TotalEpisodes > 0 || len(local.MissingEpisodes) > 0
}

func subscriptionSearchResultText(item SearchResult) string {
	return strings.TrimSpace(strings.Join([]string{item.Title, item.Subtitle, item.Labels}, " "))
}

func selectRSSSubscriptionCandidates(items []rssItem, sub *model.Subscription, filter *regexp.Regexp, seenSet map[string]struct{}, local LocalAvailability) []siteSearchCandidate {
	if seenSet == nil {
		seenSet = map[string]struct{}{}
	}
	candidates := make([]siteSearchCandidate, 0, len(items))
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		if filter != nil && !filter.MatchString(title) {
			continue
		}
		searchItem := SearchResult{Title: title}
		if !matchesSubscriptionRules(sub, title) || !matchesSubscriptionTorrentRules(sub, searchItem) {
			continue
		}
		download := strings.TrimSpace(item.Enclosure.URL)
		if download == "" {
			download = strings.TrimSpace(item.Link)
		}
		if download == "" {
			continue
		}
		guid := stableRSSItemGUID(title, item.GUID, item.Link, item.Enclosure.URL)
		if _, ok := seenSet[guid]; ok {
			continue
		}
		searchItem.DownloadURL = download
		refs := subscriptionCandidateEpisodeRefs(sub, title)
		season, episode := ParseEpisode(title)
		if episode <= 0 && len(refs) > 0 {
			season = refs[0].Season
			episode = refs[0].Episode
		}
		episodes := episodeNumbersFromRefs(refs, season)
		candidates = append(candidates, siteSearchCandidate{
			Item:     searchItem,
			Download: download,
			GUID:     guid,
			Season:   season,
			Episode:  episode,
			Episodes: episodes,
			Pack:     isSeriesPackTitle(title) || len(episodes) > 1,
			Score:    subscriptionCandidateScore(sub, searchItem),
		})
	}
	return selectPreparedSubscriptionCandidates(candidates, sub, local)
}

func subscriptionCandidateEpisodeRefs(sub *model.Subscription, text string) []episodeRef {
	if refs := episodeRefsFromTitle(text); len(refs) > 0 {
		return refs
	}
	if sub == nil || !isSubscriptionSeriesType(sub.MediaType) || isSeriesPackTitle(text) || patSeasonOnly.MatchString(text) {
		return nil
	}
	episode := inferLooseSubscriptionEpisode(maskSubscriptionTitleQueries(sub, text))
	if episode <= 0 {
		return nil
	}
	return []episodeRef{{Season: 1, Episode: episode}}
}

func maskSubscriptionTitleQueries(sub *model.Subscription, text string) string {
	if sub == nil || strings.TrimSpace(text) == "" {
		return text
	}
	out := text
	outFold := strings.ToLower(out)
	for _, query := range subscriptionTitleMatchQueries(sub) {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		queryFold := strings.ToLower(query)
		for {
			idx := strings.Index(outFold, queryFold)
			if idx < 0 {
				break
			}
			out = out[:idx] + strings.Repeat(" ", len(query)) + out[idx+len(query):]
			outFold = strings.ToLower(out)
		}
	}
	return out
}

func inferLooseSubscriptionEpisode(text string) int {
	for _, match := range looseSubscriptionEpisodeRE.FindAllStringSubmatchIndex(text, -1) {
		if len(match) < 4 || match[2] < 0 || match[3] < 0 {
			continue
		}
		if isDecimalFractionMatch(text, match[2]) {
			continue
		}
		value, err := strconv.Atoi(text[match[2]:match[3]])
		if err != nil || !looksLikeLooseEpisodeNumber(value) {
			continue
		}
		return value
	}
	return 0
}

func isDecimalFractionMatch(text string, digitStart int) bool {
	return digitStart >= 2 && text[digitStart-1] == '.' && text[digitStart-2] >= '0' && text[digitStart-2] <= '9'
}

func looksLikeLooseEpisodeNumber(value int) bool {
	switch {
	case value <= 0, value > 200:
		return false
	default:
		return true
	}
}

func episodeNumbersFromRefs(refs []episodeRef, fallbackSeason int) []int {
	if len(refs) == 0 {
		return nil
	}
	if fallbackSeason <= 0 {
		fallbackSeason = refs[0].Season
	}
	out := make([]int, 0, len(refs))
	seen := map[int]struct{}{}
	for _, ref := range refs {
		season := ref.Season
		if season <= 0 {
			season = 1
		}
		if fallbackSeason > 0 && season != fallbackSeason {
			continue
		}
		if ref.Episode <= 0 {
			continue
		}
		if _, ok := seen[ref.Episode]; ok {
			continue
		}
		seen[ref.Episode] = struct{}{}
		out = append(out, ref.Episode)
	}
	return out
}
