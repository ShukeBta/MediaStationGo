package service

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *SubscriptionService) updateSubscriptionTotalEpisodes(ctx context.Context, sub *model.Subscription, total int) {
	if s == nil || s.repo == nil || s.repo.DB == nil || sub == nil || total <= sub.TotalEpisodes {
		return
	}
	sub.TotalEpisodes = total
	_ = s.repo.DB.WithContext(ctx).Model(sub).Update("total_episodes", total).Error
}

func (service *SubscriptionService) backfillSiteSearchTotalEpisodes(ctx context.Context, sub *model.Subscription) {
	if sub == nil {
		return
	}
	rawMediaType := strings.TrimSpace(sub.MediaType)
	mediaType := normalizeMediaType(rawMediaType, sub.Name+" "+sub.Filter, "")
	seasonScopedSiteSearch := subscriptionUsesSiteSearch(sub) &&
		(rawMediaType == "" || isSubscriptionSeriesType(mediaType))
	if sub.TotalEpisodes > 0 && !seasonScopedSiteSearch {
		return
	}
	if !subscriptionShouldInferTotal(sub) && !seasonScopedSiteSearch {
		return
	}
	// A site-scoped PT subscription normally has no stable metadata identity.
	// Searching TMDb by every tracker keyword on every run only adds latency and
	// turns an optional provider outage into repeated work. Keep that subscription
	// open-ended unless the rule carries an explicit TMDb id; a generic
	// site-search subscription can still use the metadata backfill path.
	if seasonScopedSiteSearch {
		params := siteSearchParamsFromURL(sub.FeedURL)
		if params.SiteID != "" && subscriptionExplicitTMDbID(sub) <= 0 {
			return
		}
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	total := 0
	if seasonScopedSiteSearch {
		// Season scoped tracker subscriptions can identify a TV season even when
		// older rows lack a media_type value. Use metadata directly; the resolver
		// stops after TMDb for this path and does not fall back to guessed totals.
		total = service.resolveSubscriptionMetadataTotalEpisodes(lookupCtx, sub)
	} else {
		total = service.resolveSubscriptionTotalEpisodes(lookupCtx, sub, 0)
	}
	if seasonScopedSiteSearch {
		service.setSiteSearchTotalEpisodes(ctx, sub, total)
		return
	}
	service.updateSubscriptionTotalEpisodes(ctx, sub, total)
}

func (service *SubscriptionService) setSiteSearchTotalEpisodes(ctx context.Context, sub *model.Subscription, total int) {
	if service == nil || service.repo == nil || service.repo.DB == nil || sub == nil || total <= 0 || total == sub.TotalEpisodes {
		return
	}
	sub.TotalEpisodes = total
	_ = service.repo.DB.WithContext(ctx).Model(sub).Update("total_episodes", total).Error
}

func inferRSSTotalEpisodes(items []rssItem, sub *model.Subscription, filter *regexp.Regexp, defaultExcludeWords ...string) int {
	if !subscriptionShouldInferTotal(sub) {
		return 0
	}
	maxEpisode := 0
	for _, item := range items {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			continue
		}
		if filter != nil && !filter.MatchString(title) {
			continue
		}
		if !subscriptionTitleMatchesQuery(sub, title) {
			continue
		}
		if !matchesSubscriptionRules(sub, title, defaultExcludeWords...) {
			continue
		}
		_, episode := ParseEpisode(title)
		if episode > maxEpisode {
			maxEpisode = episode
		}
	}
	return maxEpisode
}

func inferSearchTotalEpisodes(results []SearchResult, sub *model.Subscription, defaultExcludeWords ...string) int {
	if !subscriptionShouldInferTotal(sub) {
		return 0
	}
	maxEpisode := 0
	for _, item := range results {
		matchText := subscriptionSearchResultText(item)
		if !subscriptionSearchResultMatchesQuery(sub, item) {
			continue
		}
		if !matchesSubscriptionRules(sub, matchText, defaultExcludeWords...) {
			continue
		}
		_, episode := ParseEpisode(matchText)
		if episode > maxEpisode {
			maxEpisode = episode
		}
	}
	return maxEpisode
}

func subscriptionShouldInferTotal(sub *model.Subscription) bool {
	if sub == nil {
		return false
	}
	mediaType := normalizeMediaType(sub.MediaType, sub.Name+" "+sub.Filter, "")
	return isSubscriptionSeriesType(mediaType)
}

func (s *SubscriptionService) resolveSubscriptionTotalEpisodes(ctx context.Context, sub *model.Subscription, fallback int) int {
	if !subscriptionShouldInferTotal(sub) {
		return 0
	}
	if sub.TotalEpisodes > 0 {
		return sub.TotalEpisodes
	}
	if total := s.resolveSubscriptionMetadataTotalEpisodes(ctx, sub); total > 0 {
		return total
	}
	return fallback
}

func (s *SubscriptionService) resolveSubscriptionMetadataTotalEpisodes(ctx context.Context, sub *model.Subscription) int {
	if s == nil || s.scraper == nil || sub == nil {
		return 0
	}
	queries := subscriptionEpisodeMetadataQueries(sub)

	// Priority: TMDb -> Douban -> Bangumi -> TheTVDB -> Fanart -> title fallback.
	// Fanart.tv is artwork-only in MediaStationGo, so it intentionally does not
	// claim episode counts and lets the title fallback handle the final layer.
	if s.scraper.tmdb != nil {
		if id := subscriptionExplicitTMDbID(sub); id > 0 {
			if total, err := s.resolveSubscriptionTMDbTotalEpisodes(ctx, sub, id); err == nil && total > 0 {
				return total
			} else if err != nil && s.log != nil {
				s.log.Debug("subscription tmdb episode count failed", zap.Int("tmdb_id", id), zap.Error(err))
			}
		}
		for _, query := range queries {
			match, err := s.scraper.tmdb.SearchTV(ctx, query, 0)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription tmdb search failed", zap.String("query", query), zap.Error(err))
				}
				continue
			}
			if match == nil || match.TMDbID <= 0 {
				continue
			}
			total, err := s.resolveSubscriptionTMDbTotalEpisodes(ctx, sub, match.TMDbID)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription tmdb episode count failed", zap.Int("tmdb_id", match.TMDbID), zap.Error(err))
				}
				continue
			}
			if total > 0 {
				return total
			}
		}
	}
	if subscriptionUsesSiteSearch(sub) {
		return 0
	}

	if s.scraper.douban != nil {
		for _, query := range queries {
			total, err := s.scraper.douban.GetEpisodeCount(ctx, query)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription douban episode count failed", zap.String("query", query), zap.Error(err))
				}
				continue
			}
			if total > 0 {
				return total
			}
		}
	}

	if s.scraper.bangumi != nil {
		for _, query := range queries {
			match, err := s.scraper.bangumi.Search(ctx, query)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription bangumi search failed", zap.String("query", query), zap.Error(err))
				}
				continue
			}
			if match == nil || match.BangumiID <= 0 {
				continue
			}
			total, err := s.scraper.bangumi.GetEpisodeCount(ctx, match.BangumiID)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription bangumi episode count failed", zap.Int("bangumi_id", match.BangumiID), zap.Error(err))
				}
				continue
			}
			if total > 0 {
				return total
			}
		}
	}

	if s.scraper.thetvdb != nil {
		for _, query := range queries {
			match, err := s.scraper.thetvdb.SearchSeries(ctx, query)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription thetvdb search failed", zap.String("query", query), zap.Error(err))
				}
				continue
			}
			if match == nil || strings.TrimSpace(match.TheTVDBID) == "" {
				continue
			}
			total, err := s.scraper.thetvdb.GetSeriesEpisodeCount(ctx, match.TheTVDBID)
			if err != nil {
				if s.log != nil {
					s.log.Debug("subscription thetvdb episode count failed", zap.String("thetvdb_id", match.TheTVDBID), zap.Error(err))
				}
				continue
			}
			if total > 0 {
				return total
			}
		}
	}

	return 0
}

func (service *SubscriptionService) resolveSubscriptionTMDbTotalEpisodes(ctx context.Context, sub *model.Subscription, tmdbID int) (int, error) {
	if !subscriptionUsesSiteSearch(sub) {
		return service.scraper.tmdb.GetTVEpisodeCount(ctx, tmdbID)
	}
	episodes, err := service.scraper.tmdb.GetTVSeasonEpisodeDetails(ctx, tmdbID, subscriptionSeasonNumber(sub))
	if err != nil {
		return 0, err
	}
	for episode := 1; episode <= len(episodes); episode++ {
		if _, exists := episodes[episode]; !exists {
			return 0, nil
		}
	}
	return len(episodes), nil
}
