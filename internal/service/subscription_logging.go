package service

import (
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func subscriptionRunLogFields(sub *model.Subscription) []zap.Field {
	fields := []zap.Field{}
	if sub == nil {
		return fields
	}
	return append(fields,
		zap.String("subscription_id", sub.ID),
		zap.String("subscription", sub.Name),
		zap.String("feed_kind", subscriptionFeedKind(sub.FeedURL)),
		zap.String("filter", sub.Filter),
		zap.String("media_type", sub.MediaType),
		zap.String("media_category", sub.MediaCategory),
		zap.String("search_mode", sub.SearchMode),
		zap.Bool("enabled", sub.Enabled),
		zap.Bool("wash_enabled", sub.WashEnabled),
		zap.String("wash_priority", sub.WashPriority),
		zap.Int("total_episodes", sub.TotalEpisodes),
	)
}

func appendSubscriptionRunResultFields(fields []zap.Field, queued int, started time.Time) []zap.Field {
	return append(fields,
		zap.Int("queued", queued),
		zap.Int64("duration_ms", time.Since(started).Milliseconds()),
	)
}

func subscriptionFeedKind(feedURL string) string {
	raw := strings.TrimSpace(feedURL)
	if raw == "" {
		return "empty"
	}
	lower := strings.ToLower(raw)
	if strings.HasPrefix(lower, "site-search://") {
		return "site-search"
	}
	parsed, err := url.Parse(raw)
	if err == nil && parsed.Scheme != "" {
		return parsed.Scheme
	}
	return "unknown"
}
