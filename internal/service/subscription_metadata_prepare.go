package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *SubscriptionService) prepareSubscriptionForRun(ctx context.Context, sub *model.Subscription) {
	if s == nil || sub == nil {
		return
	}
	normalizeSubscriptionDefaults(sub)
	updates := map[string]any{}
	// Tracker subscriptions can run entirely from saved metadata and title
	// rules; downloading must not wait on optional metadata providers.
	if !subscriptionUsesSiteSearch(sub) {
		s.fillSubscriptionRunMetadata(ctx, sub, updates)
	}
	if identityKey := model.SubscriptionIdentityKey(sub); identityKey != sub.IdentityKey {
		sub.IdentityKey = identityKey
		updates["identity_key"] = identityKey
	}
	if len(updates) > 0 && s.repo != nil && s.repo.DB != nil {
		if err := s.repo.DB.WithContext(ctx).Model(&model.Subscription{}).Where("id = ?", sub.ID).Updates(updates).Error; err != nil && s.log != nil {
			s.log.Debug("subscription metadata prepare persist failed", zap.String("id", sub.ID), zap.Error(err))
		}
	}
}

func (s *SubscriptionService) fillSubscriptionRunMetadata(ctx context.Context, sub *model.Subscription, updates map[string]any) {
	if sub == nil {
		return
	}
	if needsSubscriptionMetadataLookup(sub) {
		query := subscriptionMetadataPrepareQuery(sub)
		lookupCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		defer cancel()
		if match := s.lookupSubscriptionMetadata(lookupCtx, strings.TrimSpace(sub.MediaType), query, sub); match != nil {
			applySubscriptionMetadataMatch(sub, match, updates)
		}
	}
	if !subscriptionUsesResourceImport(sub) && isSubscriptionSeriesType(strings.TrimSpace(sub.MediaType)) && sub.TotalEpisodes <= 0 {
		if total := s.resolveSubscriptionTotalEpisodes(ctx, sub, 0); total > 0 {
			sub.TotalEpisodes = total
			updates["total_episodes"] = total
		}
	}
}

func needsSubscriptionMetadataLookup(sub *model.Subscription) bool {
	if sub == nil {
		return false
	}
	return strings.TrimSpace(sub.MediaType) == "" ||
		strings.TrimSpace(sub.OriginalName) == "" ||
		sub.Year <= 0
}

func subscriptionMetadataPrepareQuery(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	if value := strings.TrimSpace(sub.Filter); value != "" {
		return value
	}
	return strings.TrimSpace(sub.Name)
}

func applySubscriptionMetadataMatch(sub *model.Subscription, match *Match, updates map[string]any) {
	if sub == nil || match == nil {
		return
	}
	if strings.TrimSpace(sub.MediaType) == "" {
		if mediaType := normalizeMetadataMatchSubscriptionType(match); mediaType != "" {
			sub.MediaType = mediaType
			updates["media_type"] = mediaType
		}
	}
	if strings.TrimSpace(sub.OriginalName) == "" {
		if value := strings.TrimSpace(match.OriginalName); value != "" {
			sub.OriginalName = value
			updates["original_name"] = value
		}
	}
	if sub.Year <= 0 && match.Year > 0 {
		sub.Year = match.Year
		updates["year"] = match.Year
	}
}

func normalizeMetadataMatchSubscriptionType(match *Match) string {
	if match == nil {
		return ""
	}
	switch normalizeOrganizeMediaType(match.MediaType) {
	case "movie":
		return "movie"
	case "tv":
		return "tv"
	case "anime":
		return "anime"
	case "variety":
		return "variety"
	case "adult":
		return "adult"
	default:
		return ""
	}
}

// Older tracker subscriptions may have no media type. Explicit episode
// markers and tracker categories provide enough evidence without a lookup.
func (s *SubscriptionService) preparePTSubscriptionFromResults(ctx context.Context, sub *model.Subscription, results []SearchResult) {
	if sub == nil || strings.TrimSpace(sub.MediaType) != "" {
		return
	}
	for _, item := range results {
		if !subscriptionSearchResultMatchesQuery(sub, item) {
			continue
		}
		mediaType := normalizeMediaType("", subscriptionSearchResultText(item), item.Category)
		if sub.MediaType == "" || isSubscriptionSeriesType(mediaType) {
			sub.MediaType = mediaType
		}
		if isSubscriptionSeriesType(mediaType) {
			break
		}
	}
	if sub.MediaType == "" {
		return
	}
	model.RefreshSubscriptionIdentity(sub)
	if err := s.repo.DB.WithContext(ctx).Model(sub).Updates(map[string]any{
		"media_type": sub.MediaType, "identity_key": sub.IdentityKey,
	}).Error; err != nil && s.log != nil {
		s.log.Warn("tracker subscription type update failed", zap.String("id", sub.ID), zap.Error(err))
	}
}
