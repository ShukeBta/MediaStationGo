package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *SubscriptionService) runSiteSearch(ctx context.Context, sub *model.Subscription) (int, error) {
	if s.site == nil {
		if s.log != nil {
			s.log.Warn("site-search subscription service unavailable", subscriptionSiteSearchLogFields(sub, "")...)
		}
		return 0, errors.New("site search service unavailable")
	}
	keywords := siteSearchKeywords(sub)
	keyword := ""
	if len(keywords) > 0 {
		keyword = keywords[0]
	}
	if keyword == "" {
		if s.log != nil {
			s.log.Warn("site-search subscription keyword missing", subscriptionSiteSearchLogFields(sub, "")...)
		}
		return 0, errors.New("site-search subscription keyword required")
	}
	if s.log != nil {
		s.log.Info("site-search subscription run started", subscriptionSiteSearchLogFields(sub, keyword)...)
	}

	results, err := s.searchSubscriptionSites(ctx, sub, keywords)
	if err != nil {
		return 0, err
	}
	if len(results) == 0 {
		return s.finishSiteSearchNoResults(sub, keyword)
	}
	s.updateSubscriptionTotalEpisodes(ctx, sub, s.resolveSubscriptionTotalEpisodes(ctx, sub, inferSearchTotalEpisodes(results, sub)))

	guidKey, seen, seenSet := s.loadSiteSearchSeen(ctx, sub)
	availability := mergeLocalAvailability(
		SubscriptionLocalAvailability(ctx, s.repo, sub),
		s.pendingDownloadAvailability(ctx, sub),
	)
	candidates, selectionStats := selectSiteSearchCandidatesWithStats(results, sub, seenSet, availability)
	if s.log != nil {
		fields := subscriptionSiteSearchLogFields(sub, keyword)
		fields = appendSiteSearchSelectionLogFields(fields, selectionStats)
		fields = appendAvailabilityLogFields(fields, availability)
		s.log.Info("site-search subscription selection summary", fields...)
	}
	runState := &siteSearchRunState{
		Keyword:      keyword,
		Seen:         seen,
		SeenSet:      seenSet,
		Availability: availability,
	}
	queueResult := s.enqueueSiteSearchCandidates(ctx, sub, candidates, runState)
	availability = s.finishSiteSearchRun(ctx, sub, guidKey, runState)
	return s.handleSiteSearchQueueResult(sub, keyword, queueResult, selectionStats, availability)
}

func (s *SubscriptionService) finishSiteSearchRun(ctx context.Context, sub *model.Subscription, guidKey string, state *siteSearchRunState) LocalAvailability {
	availability := s.finalizePendingAvailability(sub, state.Availability)
	seen := trimSiteSearchSeen(state.Seen)
	if err := s.repo.Setting.Set(ctx, guidKey, strings.Join(seen, "\n")); err != nil && s.log != nil {
		s.log.Warn("site-search subscription seen state update failed",
			zap.String("subscription_id", sub.ID),
			zap.String("subscription", sub.Name),
			zap.Error(err))
	}
	now := time.Now()
	if err := s.repo.DB.Model(sub).Updates(map[string]any{"last_run_at": &now}).Error; err != nil && s.log != nil {
		s.log.Warn("site-search subscription last_run_at update failed",
			zap.String("subscription_id", sub.ID),
			zap.String("subscription", sub.Name),
			zap.Error(err))
	}
	if err := s.archiveCompletedSubscription(ctx, sub, availability); err != nil && s.log != nil {
		s.log.Warn("site-search subscription archive check failed",
			zap.String("subscription_id", sub.ID),
			zap.String("subscription", sub.Name),
			zap.Error(err))
	}
	return availability
}

func (s *SubscriptionService) handleSiteSearchQueueResult(sub *model.Subscription, keyword string, queueResult siteSearchQueueResult, selectionStats siteSearchSelectionStats, availability LocalAvailability) (int, error) {
	if queueResult.Queued > 0 {
		if s.log != nil {
			fields := subscriptionSiteSearchLogFields(sub, keyword)
			fields = appendSiteSearchSelectionLogFields(fields, selectionStats)
			fields = appendAvailabilityLogFields(fields, availability)
			fields = append(fields,
				zap.Int("queued", queueResult.Queued),
				zap.Strings("resources", queueResult.Resources),
				zap.Bool("archived", sub.ArchivedAt != nil),
				zap.String("archive_reason", sub.ArchiveReason),
			)
			s.log.Info("site-search subscription queued resources", fields...)
		}
		s.hub.Publish("subscription", map[string]any{
			"id":        sub.ID,
			"name":      sub.Name,
			"queued":    queueResult.Queued,
			"keyword":   keyword,
			"resources": queueResult.Resources,
		})
		s.notifySubscriptionHit(sub, queueResult.Queued, queueResult.Resources)
		return queueResult.Queued, nil
	}
	if queueResult.LastEnqueueErr != nil {
		return 0, fmt.Errorf("找到 PT 资源但加入下载器失败: %w", queueResult.LastEnqueueErr)
	}
	if s.log != nil {
		fields := subscriptionSiteSearchLogFields(sub, keyword)
		fields = appendSiteSearchSelectionLogFields(fields, selectionStats)
		fields = appendAvailabilityLogFields(fields, availability)
		fields = append(fields, zap.Int("queued", queueResult.Queued))
		s.log.Info("site-search subscription no candidate queued", fields...)
	}
	return 0, nil
}

func (s *SubscriptionService) searchSubscriptionSites(ctx context.Context, sub *model.Subscription, keywords []string) ([]SearchResult, error) {
	var (
		results       []SearchResult
		lastSearchErr error
		searchErrors  int
	)
	for _, searchKeyword := range keywords {
		found, err := s.site.Search(ctx, searchKeyword)
		if err != nil {
			lastSearchErr = err
			searchErrors++
			if s.log != nil {
				fields := subscriptionSiteSearchLogFields(sub, searchKeyword)
				fields = append(fields, zap.Error(err))
				s.log.Warn("site-search subscription search failed", fields...)
			}
			if subscriptionSiteSearchShouldStopOnError(err) {
				return nil, err
			}
			continue
		}
		results = append(results, found...)
		if hasUsableSubscriptionSearchResult(found, sub) {
			break
		}
	}
	results = dedupeSiteSearchResults(results)
	if len(results) == 0 && lastSearchErr != nil && searchErrors == len(keywords) {
		return nil, lastSearchErr
	}
	return results, nil
}

func subscriptionSiteSearchShouldStopOnError(err error) bool {
	if err == nil {
		return false
	}
	var limited *siteAPIRateLimitError
	if errors.As(err, &limited) {
		return true
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "rate limit") ||
		strings.Contains(msg, "quota") ||
		strings.Contains(msg, "too many requests") ||
		strings.Contains(msg, "status 429") ||
		strings.Contains(msg, "context deadline exceeded") ||
		strings.Contains(msg, "tls handshake timeout") ||
		strings.Contains(msg, "i/o timeout") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "temporary failure")
}

func hasUsableSubscriptionSearchResult(results []SearchResult, sub *model.Subscription) bool {
	for _, item := range results {
		if !subscriptionSearchResultMatchesQuery(sub, item) {
			continue
		}
		if !matchesSubscriptionRules(sub, subscriptionSearchResultText(item)) {
			continue
		}
		if strings.TrimSpace(firstNonEmpty(item.DownloadURL, item.TorrentURL)) != "" {
			return true
		}
	}
	return false
}

func (s *SubscriptionService) finishSiteSearchNoResults(sub *model.Subscription, keyword string) (int, error) {
	if s.log != nil {
		fields := subscriptionSiteSearchLogFields(sub, keyword)
		fields = append(fields, zap.Int("results_count", 0))
		s.log.Info("site-search subscription no results", fields...)
	}
	now := time.Now()
	if err := s.repo.DB.Model(sub).Updates(map[string]any{"last_run_at": &now}).Error; err != nil && s.log != nil {
		s.log.Warn("site-search subscription last_run_at update failed",
			zap.String("subscription_id", sub.ID),
			zap.String("subscription", sub.Name),
			zap.Error(err))
	}
	return 0, nil
}

func (s *SubscriptionService) loadSiteSearchSeen(ctx context.Context, sub *model.Subscription) (string, []string, map[string]struct{}) {
	guidKey := fmt.Sprintf("subscription.%s.seen", sub.ID)
	seenRaw, _ := s.repo.Setting.Get(ctx, guidKey)
	seen := splitNonEmpty(seenRaw)
	seenSet := make(map[string]struct{}, len(seen))
	for _, g := range seen {
		seenSet[g] = struct{}{}
	}
	return guidKey, seen, seenSet
}

func trimSiteSearchSeen(seen []string) []string {
	if len(seen) <= 200 {
		return seen
	}
	return seen[len(seen)-200:]
}
