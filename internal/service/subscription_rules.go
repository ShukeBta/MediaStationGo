package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func matchesSubscriptionRules(sub *model.Subscription, title string) bool {
	titleFold := strings.ToLower(title)
	if containsAnyExcludeToken(titleFold, defaultExcludeWords) {
		return false
	}
	if sub == nil {
		return true
	}
	if compatibilityExcludes := defaultCompatibilityExcludesForSubscription(sub); compatibilityExcludes != "" && containsAnyExcludeToken(titleFold, compatibilityExcludes) {
		return false
	}
	if sub.ExcludeWords != "" && containsAnyExcludeToken(titleFold, sub.ExcludeWords) {
		return false
	}
	if sub.ReleaseGroups != "" && !containsAnyToken(titleFold, sub.ReleaseGroups) {
		return false
	}
	if sub.Resolution != "" && sub.Resolution != "best" && !titleMatchesResolution(titleFold, sub.Resolution) {
		return false
	}
	if sub.Quality != "" && sub.Quality != "best" && !titleMatchesQuality(titleFold, sub.Quality) {
		return false
	}
	if sub.Effects != "" && !containsAnyEffect(titleFold, sub.Effects) {
		return false
	}
	return true
}

func defaultCompatibilityExcludesForSubscription(sub *model.Subscription) string {
	if sub == nil {
		return defaultCompatibilityExcludeWords
	}
	requested := strings.ToLower(strings.Join([]string{sub.Effects, sub.Quality}, ","))
	if strings.TrimSpace(requested) == "" {
		return defaultCompatibilityExcludeWords
	}
	tokens := excludeWordTokens(defaultCompatibilityExcludeWords)
	out := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == "" || compatibilityTokenRequested(requested, token) {
			continue
		}
		out = append(out, token)
	}
	return strings.Join(out, ",")
}

func compatibilityTokenRequested(requested, token string) bool {
	switch token {
	case "dovi", "dv", "dolby vision", "杜比视界":
		return containsAnyEffect(requested, "dolby-vision") || containsAnyToken(requested, "dovi,dv,dolby vision,杜比视界")
	case "dolby", "杜比":
		return containsAnyEffect(requested, "dolby-vision") || containsAnyToken(requested, "dolby,dolby vision,杜比,杜比视界,atmos,dolby atmos,杜比全景声")
	case "atmos":
		return containsAnyToken(requested, "atmos,dolby atmos,杜比全景声")
	case "h265", "h.265", "h-265", "h_265", "h 265", "hevc", "x265":
		return containsAnyToken(requested, "h265,h.265,h-265,h_265,h 265,hevc,x265")
	case "10bit", "10-bit", "10 bit", "hi10p":
		return containsAnyToken(requested, "10bit,10-bit,10 bit,hi10p")
	case "truehd", "ddp", "dd+", "eac3":
		return containsAnyToken(requested, "truehd,ddp,dd+,eac3")
	default:
		return containsAnyToken(requested, token)
	}
}

func isSubscriptionSeriesType(mediaType string) bool {
	switch normalizeMediaType(mediaType, "", "") {
	case "tv", "anime", "variety":
		return true
	default:
		return false
	}
}

func subscriptionAllowsWash(sub *model.Subscription) bool {
	if sub == nil || !sub.WashEnabled {
		return false
	}
	return subscriptionHasExplicitUpgradeCriteria(sub)
}

func subscriptionHasExplicitUpgradeCriteria(sub *model.Subscription) bool {
	if sub == nil {
		return false
	}
	if value := strings.TrimSpace(strings.ToLower(sub.Resolution)); value != "" && value != "best" {
		return true
	}
	if value := strings.TrimSpace(strings.ToLower(sub.Quality)); value != "" && value != "best" {
		return true
	}
	return strings.TrimSpace(sub.Effects) != "" ||
		strings.TrimSpace(sub.ReleaseGroups) != ""
}
