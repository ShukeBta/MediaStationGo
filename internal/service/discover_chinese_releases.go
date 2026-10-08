package service

import (
	"slices"
	"strings"
	"time"
)

func chineseReleaseDateRange(now time.Time, upcoming bool) (string, string) {
	now = now.UTC()
	if upcoming {
		return now.AddDate(0, 0, 1).Format(time.DateOnly), now.AddDate(1, 0, 0).Format(time.DateOnly)
	}
	return now.AddDate(-1, 0, 0).Format(time.DateOnly), now.Format(time.DateOnly)
}

// FilterChineseReleaseRecommendations checks fresh and stale cache results
// against today's range. Apply it after the source window has been sliced so
// removing an invalid item does not consume the next page's probe item.
func FilterChineseReleaseRecommendations(key string, items []ExternalMediaResult, now time.Time) []ExternalMediaResult {
	upcoming := false
	switch strings.TrimPrefix(key, "tmdb_") {
	case "chinese_latest_movie", "chinese_latest_tv":
	case "chinese_upcoming_movie", "chinese_upcoming_tv":
		upcoming = true
	default:
		return items
	}
	from, through := chineseReleaseDateRange(now, upcoming)
	out := make([]ExternalMediaResult, 0, len(items))
	for _, item := range items {
		date := strings.TrimSpace(item.ReleaseDate)
		if _, err := time.Parse(time.DateOnly, date); err != nil || date < from || date > through {
			continue
		}
		if len(item.Countries) > 0 && !hasChineseOriginCountry(item.Countries) {
			continue
		}
		out = append(out, item)
	}
	slices.SortStableFunc(out, func(a, b ExternalMediaResult) int {
		order := strings.Compare(a.ReleaseDate, b.ReleaseDate)
		if upcoming {
			return order
		}
		return -order
	})
	return out
}

func hasChineseOriginCountry(countries []string) bool {
	for _, country := range countries {
		switch strings.ToUpper(strings.TrimSpace(country)) {
		case "CN", "HK", "TW", "MO":
			return true
		}
	}
	return false
}
