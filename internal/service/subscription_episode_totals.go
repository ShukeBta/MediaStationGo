package service

import (
	"context"
	"regexp"
	"strings"

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

func inferRSSTotalEpisodes(items []rssItem, sub *model.Subscription, filter *regexp.Regexp) int {
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
		if !matchesSubscriptionRules(sub, title) {
			continue
		}
		_, episode := ParseEpisode(title)
		if episode > maxEpisode {
			maxEpisode = episode
		}
	}
	return maxEpisode
}

func inferSearchTotalEpisodes(results []SearchResult, sub *model.Subscription) int {
	if !subscriptionShouldInferTotal(sub) {
		return 0
	}
	maxEpisode := 0
	for _, item := range results {
		matchText := subscriptionSearchResultText(item)
		if !subscriptionSearchResultMatchesQuery(sub, item) {
			continue
		}
		if !matchesSubscriptionRules(sub, matchText) {
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
			if total, err := s.scraper.tmdb.GetTVEpisodeCount(ctx, id); err == nil && total > 0 {
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
			total, err := s.scraper.tmdb.GetTVEpisodeCount(ctx, match.TMDbID)
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
