package service

import (
	"context"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func subscriptionSiteSearchLogFields(sub *model.Subscription, keyword string) []zap.Field {
	fields := []zap.Field{zap.String("keyword", keyword), zap.Strings("search_keywords", siteSearchKeywords(sub))}
	if sub == nil {
		return fields
	}
	fields = append(fields,
		zap.String("subscription_id", sub.ID),
		zap.String("subscription", sub.Name),
		zap.String("filter", sub.Filter),
		zap.String("media_type", sub.MediaType),
		zap.String("media_category", sub.MediaCategory),
		zap.String("search_mode", sub.SearchMode),
		zap.String("imdb_id", sub.IMDBID),
		zap.Bool("wash_enabled", sub.WashEnabled),
		zap.String("wash_priority", sub.WashPriority),
		zap.Int("total_episodes", sub.TotalEpisodes),
	)
	return fields
}

func appendSiteSearchSelectionLogFields(fields []zap.Field, stats siteSearchSelectionStats) []zap.Field {
	return append(fields,
		zap.Int("results_count", stats.Total),
		zap.Int("query_mismatch_count", stats.QueryMismatch),
		zap.Strings("query_mismatch_examples", stats.QueryMismatchExamples),
		zap.Int("relaxed_query_match_count", stats.RelaxedQueryMatch),
		zap.Int("rule_mismatch_count", stats.RuleMismatch),
		zap.Int("missing_download_count", stats.MissingDownload),
		zap.Int("seen_count", stats.Seen),
		zap.Int("prepared_count", stats.Prepared),
		zap.Int("selected_count", stats.Selected),
		zap.Bool("local_already_satisfied", stats.LocalAlreadySatisfied),
		zap.Bool("local_series_pack_present", stats.LocalSeriesPackPresent),
		zap.Bool("series_complete", stats.SeriesComplete),
		zap.Int("existing_episode_skipped_count", stats.ExistingEpisodeSkipped),
		zap.Int("not_missing_episode_skipped_count", stats.NotMissingEpisodeSkipped),
		zap.Int("no_episode_skipped_count", stats.NoEpisodeSkipped),
		zap.Bool("pack_fallback_available", stats.PackFallbackAvailable),
		zap.Bool("pack_fallback_used", stats.PackFallbackUsed),
	)
}

func appendAvailabilityLogFields(fields []zap.Field, availability LocalAvailability) []zap.Field {
	missingSample, missingMore := limitedEpisodeSample(availability.MissingEpisodes, 20)
	return append(fields,
		zap.Int("local_media_count", availability.LocalMediaCount),
		zap.Bool("in_library", availability.InLibrary),
		zap.Bool("has_series_pack", availability.HasSeriesPack),
		zap.Int("downloaded_episodes", availability.DownloadedEpisodes),
		zap.Int("availability_total_episodes", availability.TotalEpisodes),
		zap.Int("missing_episode_count", len(availability.MissingEpisodes)),
		zap.Ints("missing_episodes", missingSample),
		zap.Int("missing_episodes_more", missingMore),
	)
}

func limitedEpisodeSample(values []int, limit int) ([]int, int) {
	if limit <= 0 || len(values) == 0 {
		return nil, len(values)
	}
	if len(values) <= limit {
		out := append([]int(nil), values...)
		return out, 0
	}
	out := append([]int(nil), values[:limit]...)
	return out, len(values) - limit
}

func (s *SubscriptionService) shouldSkipExistingTorrent(ctx context.Context, mediaType string, candidate siteSearchCandidate) bool {
	if s == nil || s.downloads == nil {
		return false
	}
	if isSubscriptionSeriesType(mediaType) && candidate.Episode > 0 {
		return false
	}
	return s.downloads.TorrentExistsByName(ctx, candidate.Item.Title)
}

func siteSearchKeywords(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	values := make([]string, 0, 8)
	if strings.EqualFold(strings.TrimSpace(sub.SearchMode), "imdb") && strings.TrimSpace(sub.IMDBID) != "" {
		values = append(values, strings.TrimSpace(sub.IMDBID))
	}
	if u, err := url.Parse(sub.FeedURL); err == nil {
		if keyword := strings.TrimSpace(u.Query().Get("keyword")); keyword != "" {
			values = append(values, keyword)
		}
	}
	if strings.TrimSpace(sub.Filter) != "" {
		values = append(values, sub.Filter)
	}
	if len(values) == 0 && strings.TrimSpace(sub.Name) != "" {
		values = append(values, sub.Name)
	}
	values = append(values, subscriptionFeedAliases(sub)...)
	values = append(values, subscriptionMetadataAliases(sub)...)
	for _, value := range append([]string(nil), values...) {
		if cleaned := cleanAvailabilityTitle(value); cleaned != "" {
			values = append(values, cleaned)
		}
	}
	return compactUniqueStrings(values...)
}

func siteSearchKeyword(sub *model.Subscription) string {
	keywords := siteSearchKeywords(sub)
	if len(keywords) == 0 {
		return ""
	}
	return keywords[0]
}

func subscriptionFeedAliases(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	u, err := url.Parse(sub.FeedURL)
	if err != nil {
		return nil
	}
	q := u.Query()
	values := make([]string, 0, len(q["alias"])+2)
	values = append(values, q["alias"]...)
	for _, raw := range q["aliases"] {
		for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
			return r == '|' || r == '\n' || r == '\r' || r == '\t'
		}) {
			values = append(values, part)
		}
	}
	return compactUniqueStrings(values...)
}

func subscriptionMetadataAliases(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	title := cleanAvailabilityTitle(firstNonEmpty(sub.Filter, sub.Name))
	return buildSubscribeAliases(title, sub.OriginalName, sub.Year)
}

func compactUniqueStrings(values ...string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeAvailabilityComparable(value)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func dedupeSiteSearchResults(results []SearchResult) []SearchResult {
	if len(results) < 2 {
		return results
	}
	seen := make(map[string]int, len(results))
	out := make([]SearchResult, 0, len(results))
	for _, item := range results {
		download := strings.TrimSpace(item.DownloadURL)
		if download == "" {
			download = strings.TrimSpace(item.TorrentURL)
		}
		key := stableSiteSearchGUID(item, download)
		if idx, ok := seen[key]; ok {
			out[idx].SearchKeyword = mergeSearchResultTextHints(out[idx].SearchKeyword, item.SearchKeyword)
			out[idx].Subtitle = mergeSearchResultTextHints(out[idx].Subtitle, item.Subtitle)
			continue
		}
		seen[key] = len(out)
		out = append(out, item)
	}
	return out
}

func mergeSearchResultTextHints(current, next string) string {
	return strings.Join(compactUniqueStrings(current, next), "\n")
}
