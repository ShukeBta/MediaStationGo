package service

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func subscriptionTitleMatchesQuery(sub *model.Subscription, title string) bool {
	if strings.TrimSpace(title) == "" {
		return false
	}
	for _, query := range subscriptionTitleMatchQueries(sub) {
		if strings.Contains(normalizeAvailabilityComparable(title), normalizeAvailabilityComparable(query)) {
			return true
		}
	}
	return len(subscriptionTitleMatchQueries(sub)) == 0
}

func subscriptionSearchResultMatchesQuery(sub *model.Subscription, item SearchResult) bool {
	if subscriptionNeedsExactKeywordTitleMatch(sub) {
		return subscriptionExactKeywordTitleMatchesResult(sub, item)
	}
	matchText := subscriptionSearchResultText(item)
	if subscriptionTitleMatchesQuery(sub, matchText) {
		return true
	}
	if !subscriptionSearchResultYearCompatible(sub, matchText) {
		return false
	}
	return subscriptionKeywordMatchesQuery(sub, item.SearchKeyword)
}

// subscriptionNeedsExactKeywordTitleMatch limits the release-title guard to
// tracker keyword subscriptions without a provider identity. RSS/manual
// download flows do not use this matcher, and a subscription with a TMDb id
// keeps the existing title/keyword matching behavior.
func subscriptionNeedsExactKeywordTitleMatch(sub *model.Subscription) bool {
	if sub == nil || !subscriptionUsesSiteSearch(sub) || sub.TMDbID > 0 || subscriptionExplicitTMDbID(sub) > 0 {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(sub.SearchMode))
	return mode == "" || mode == "keyword"
}

// subscriptionExactKeywordTitleMatchesResult parses the release title before
// comparing it with the subscription's complete title. A query such as
// "谷雨" therefore cannot match the distinct full title
// "倾世锦鳞谷雨来" merely because it is contained in that title.
// The primary release title is authoritative. A subtitle or label cannot turn
// a different primary title into a match; this keeps a longer release title
// from being accepted merely because a secondary field contains the keyword.
func subscriptionExactKeywordTitleMatchesResult(sub *model.Subscription, item SearchResult) bool {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return false
	}
	primary, parsedYear := subscriptionCandidateParsedTitleForSubscription(sub, title)
	if primary == "" {
		return false
	}
	if !subscriptionCandidateYearCompatible(sub, subscriptionSearchResultText(item), parsedYear) {
		return false
	}

	queries := subscriptionExactTitleQueries(sub)
	if len(queries) == 0 {
		return false
	}
	primaryKey := normalizeAvailabilityComparable(primary)
	if primaryKey == "" {
		return false
	}
	for _, query := range queries {
		if primaryKey == normalizeAvailabilityComparable(query) {
			return true
		}
	}
	return false
}

func subscriptionExactTitleQueries(sub *model.Subscription) []string {
	queries := make([]string, 0)
	for _, raw := range subscriptionTitleMatchQueries(sub) {
		if title := subscriptionCandidateParsedTitleOnly(raw); title != "" {
			queries = append(queries, title)
		}
	}
	return compactUniqueStrings(queries...)
}

func subscriptionCandidateParsedTitle(value string) (string, int) {
	parsed, year := CleanQuery(value)
	if parsed == "" {
		return "", year
	}
	return cleanSubscriptionCandidateTitle(parsed), year
}

func subscriptionCandidateParsedTitleForSubscription(sub *model.Subscription, value string) (string, int) {
	parsed, year := CleanQuery(value)
	if parsed == "" {
		return "", year
	}

	// Keep a number when the unstripped release title already equals the full
	// subscription title. For example, "Toy Story 4" is a title, not episode 4.
	for _, query := range subscriptionExactTitleQueries(sub) {
		if normalizeAvailabilityComparable(cleanSubscriptionCandidateTitle(parsed)) == normalizeAvailabilityComparable(query) {
			return cleanSubscriptionCandidateTitle(parsed), year
		}
	}

	if stripped := stripSubscriptionCandidateEpisode(sub, value); stripped != value {
		parsed, _ = CleanQuery(stripped)
	}
	return cleanSubscriptionCandidateTitle(parsed), year
}

func cleanSubscriptionCandidateTitle(parsed string) string {
	// CleanQuery removes season/episode and codec markers. The existing
	// availability/pack cleaners then remove the remaining whole-season words
	// (Complete, 全集, 整季, ...), so ordinary season-pack names remain valid.
	parsed = seriesPackRE.ReplaceAllString(parsed, " ")
	parsed = cleanAvailabilityTitle(parsed)
	return strings.TrimSpace(strings.Join(strings.Fields(parsed), " "))
}

func stripSubscriptionCandidateEpisode(sub *model.Subscription, value string) string {
	// CleanQuery handles the ordinary explicit markers. Chinese episode ranges
	// are parsed by the episode pipeline but are intentionally kept together by
	// CleanQuery, so drop the recognized range and everything after it here.
	if refs := episodeRefsFromTitle(value); len(refs) > 1 {
		rangeStart := -1
		for _, pattern := range []*regexp.Regexp{patSEnERange, patCNRange} {
			if loc := pattern.FindStringIndex(value); loc != nil && (rangeStart < 0 || loc[0] < rangeStart) {
				rangeStart = loc[0]
			}
		}
		if rangeStart >= 0 {
			return value[:rangeStart]
		}
	}

	// Bare numbers use the same query masking and episode-number validation as
	// candidate collection. Masking preserves byte offsets, so the validated
	// episode digits can be removed from the original release title.
	masked := maskSubscriptionTitleQueries(sub, value)
	if start, end, _, ok := looseSubscriptionEpisodeMatch(masked); ok {
		return value[:start] + value[end:]
	}
	return value
}

func subscriptionCandidateParsedTitleOnly(value string) string {
	title, _ := subscriptionCandidateParsedTitle(value)
	return title
}

func subscriptionCandidateYearCompatible(sub *model.Subscription, raw string, parsedYear int) bool {
	expected := subscriptionExpectedYear(sub)
	if expected <= 0 {
		return true
	}
	// Check every explicit year in the release title. Requiring all parsed
	// years to agree is conservative when a release contains multiple year
	// hints and avoids letting a matching year hide a conflicting one.
	years := titleYears(raw)
	if parsedYear > 0 {
		years = append(years, parsedYear)
	}
	if len(years) == 0 {
		return true
	}
	for _, year := range years {
		if year != expected {
			return false
		}
	}
	return true
}

func subscriptionSearchResultYearCompatible(sub *model.Subscription, title string) bool {
	expected := subscriptionExpectedYear(sub)
	if expected <= 0 {
		return true
	}
	years := titleYears(title)
	if len(years) == 0 {
		return true
	}
	for _, year := range years {
		if year == expected {
			return true
		}
	}
	return false
}

func subscriptionExpectedYear(sub *model.Subscription) int {
	if sub == nil {
		return 0
	}
	if sub.Year > 0 {
		return sub.Year
	}
	for _, value := range []string{sub.Filter, sub.Name, sub.FeedURL} {
		for _, year := range titleYears(value) {
			return year
		}
	}
	return 0
}

func titleYears(value string) []int {
	matches := regexp.MustCompile(`\b(19\d{2}|20\d{2})\b`).FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]int, 0, len(matches))
	seen := map[int]struct{}{}
	for _, match := range matches {
		year, err := strconv.Atoi(match)
		if err != nil {
			continue
		}
		if _, ok := seen[year]; ok {
			continue
		}
		seen[year] = struct{}{}
		out = append(out, year)
	}
	return out
}

func subscriptionKeywordMatchesQuery(sub *model.Subscription, keyword string) bool {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return false
	}
	normalizedKeyword := normalizeAvailabilityComparable(keyword)
	if normalizedKeyword == "" {
		return false
	}
	for _, query := range subscriptionTitleMatchQueries(sub) {
		normalizedQuery := normalizeAvailabilityComparable(query)
		if normalizedQuery == "" {
			continue
		}
		if strings.Contains(normalizedKeyword, normalizedQuery) || strings.Contains(normalizedQuery, normalizedKeyword) {
			return true
		}
	}
	return len(subscriptionTitleMatchQueries(sub)) == 0
}

func subscriptionTitleMatchQueries(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	values := []string{
		availabilityQuery(subscriptionName(sub), subscriptionFilter(sub)),
		cleanAvailabilityTitle(subscriptionFilter(sub)),
		cleanAvailabilityTitle(subscriptionName(sub)),
	}
	for _, alias := range subscriptionFeedAliases(sub) {
		values = append(values, alias, cleanAvailabilityTitle(alias))
	}
	for _, alias := range subscriptionMetadataAliases(sub) {
		values = append(values, alias, cleanAvailabilityTitle(alias))
	}
	return compactUniqueStrings(values...)
}

func subscriptionEpisodeMetadataQueries(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	raw := []string{
		siteSearchKeyword(sub),
		sub.Filter,
		sub.Name,
		availabilityQuery(subscriptionName(sub), subscriptionFilter(sub)),
	}
	out := make([]string, 0, len(raw)*2)
	for _, value := range raw {
		value = cleanAvailabilityTitle(value)
		if value == "" {
			continue
		}
		if cleaned, _ := CleanQuery(value); cleaned != "" {
			out = append(out, cleaned)
		}
		out = append(out, value)
	}
	return compactUniqueStrings(out...)
}

func subscriptionExplicitTMDbID(sub *model.Subscription) int {
	if sub == nil {
		return 0
	}
	values := []string{sub.Name, sub.Filter, sub.FeedURL}
	for _, raw := range values {
		for _, pattern := range []string{`(?i)\btmdb[_:\-\s=]+(\d{2,})`, `(?i)\btmdbid[_:\-\s=]+(\d{2,})`} {
			if m := regexp.MustCompile(pattern).FindStringSubmatch(raw); len(m) >= 2 {
				var id int
				if _, err := fmt.Sscanf(m[1], "%d", &id); err == nil && id > 0 {
					return id
				}
			}
		}
		if u, err := url.Parse(raw); err == nil {
			for _, key := range []string{"tmdb_id", "tmdb", "tmdbid"} {
				var id int
				if _, err := fmt.Sscanf(u.Query().Get(key), "%d", &id); err == nil && id > 0 {
					return id
				}
			}
		}
	}
	return 0
}
