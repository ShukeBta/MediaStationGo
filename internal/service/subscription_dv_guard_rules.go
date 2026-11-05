package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var dolbyVisionExcludeTokens = map[string]struct{}{
	"dv": {}, "dovi": {}, "dolby vision": {}, "dolby": {},
	"杜比视界": {}, "杜比": {},
}

// subscriptionEffectiveExcludeWords mirrors the exclude-word precedence used
// by matchesSubscriptionRules: compatibility defaults are applied first,
// then a subscription's words, falling back to configured defaults only when
// its own list is empty.
func subscriptionEffectiveExcludeWords(sub *model.Subscription, configuredDefault string) string {
	if sub == nil {
		return strings.TrimSpace(configuredDefault)
	}
	words := strings.TrimSpace(sub.ExcludeWords)
	if words == "" {
		words = strings.TrimSpace(configuredDefault)
	}
	compatibility := defaultCompatibilityExcludesForSubscription(sub)
	if compatibility == "" {
		return words
	}
	if words == "" {
		return compatibility
	}
	return compatibility + "," + words
}

func subscriptionDVGuardEnabled(sub *model.Subscription, configuredDefault string) bool {
	for _, token := range excludeWordTokens(subscriptionEffectiveExcludeWords(sub, configuredDefault)) {
		if _, ok := dolbyVisionExcludeTokens[token]; ok {
			return true
		}
	}
	return false
}
