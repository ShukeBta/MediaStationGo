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
	matchText := subscriptionSearchResultText(item)
	if subscriptionTitleMatchesQuery(sub, matchText) {
		return true
	}
	if !subscriptionSearchResultYearCompatible(sub, matchText) {
		return false
	}
	return subscriptionKeywordMatchesQuery(sub, item.SearchKeyword)
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
