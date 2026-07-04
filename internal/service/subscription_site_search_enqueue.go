package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type siteSearchRunState struct {
	Keyword      string
	Seen         []string
	SeenSet      map[string]struct{}
	Availability LocalAvailability
}

type siteSearchQueueResult struct {
	Queued         int
	Resources      []string
	LastEnqueueErr error
}

func (s *SubscriptionService) enqueueSiteSearchCandidates(ctx context.Context, sub *model.Subscription, candidates []siteSearchCandidate, state *siteSearchRunState) siteSearchQueueResult {
	var result siteSearchQueueResult
	for _, candidate := range candidates {
		title, err := s.enqueueSiteSearchCandidate(ctx, sub, candidate, state)
		if err != nil {
			result.LastEnqueueErr = err
			continue
		}
		if title == "" {
			continue
		}
		result.Queued++
		result.Resources = append(result.Resources, title)
	}
	return result
}

func (s *SubscriptionService) enqueueSiteSearchCandidate(ctx context.Context, sub *model.Subscription, candidate siteSearchCandidate, state *siteSearchRunState) (string, error) {
	item := candidate.Item
	matchText := subscriptionSearchResultText(item)
	mediaType, mediaCategory := s.classifySubscriptionItem(ctx, sub, matchText, item.Category)
	if s.shouldSkipExistingTorrent(ctx, mediaType, candidate) {
		state.markCandidateAvailable(candidate)
		state.markCandidateSeen(candidate.GUID)
		s.logSiteSearchCandidateSkipped(sub, state, candidate, "existing_torrent", mediaType, "", "")
		return "", nil
	}

	realURL := s.site.ResolveDownloadURL(ctx, candidate.Download)
	savePath := s.resolveSubscriptionSavePath(ctx, sub, mediaType, mediaCategory)
	if s.downloadPathHasCandidate(ctx, sub, matchText, savePath) {
		state.markCandidateAvailable(candidate)
		state.markCandidateSeen(candidate.GUID)
		s.logSiteSearchCandidateSkipped(sub, state, candidate, "download_path_has_candidate", mediaType, mediaCategory, savePath)
		return "", nil
	}

	if _, err := s.downloads.AddDownloadWithMeta(ctx, sub.UserID, realURL, savePath, DownloadTaskMeta{
		SubscriptionID:       sub.ID,
		Title:                firstNonEmpty(item.Title, sub.Name),
		PosterURL:            sub.PosterURL,
		BackdropURL:          sub.BackdropURL,
		Overview:             sub.Overview,
		MediaType:            mediaType,
		MediaCategory:        mediaCategory,
		SourceCategory:       item.Category,
		AllowExistingLibrary: subscriptionAllowsWash(sub),
	}); err != nil {
		if IsDownloadDedupError(err) {
			if s.subscriptionCandidateConfirmedAvailable(ctx, sub, candidate) {
				state.markCandidateAvailable(candidate)
				s.logSiteSearchCandidateSkipped(sub, state, candidate, "download_dedup", mediaType, mediaCategory, savePath)
				return "", nil
			}
			s.logSiteSearchCandidateSkipped(sub, state, candidate, "download_dedup_unconfirmed", mediaType, mediaCategory, savePath)
			return "", nil
		}
		s.logSiteSearchEnqueueFailed(sub, state, candidate, mediaType, mediaCategory, savePath, err)
		return "", err
	}

	state.markCandidateAvailable(candidate)
	state.markCandidateSeen(candidate.GUID)
	s.logSiteSearchCandidateQueued(sub, state, candidate, mediaType, mediaCategory, savePath)
	return item.Title, nil
}

func (state *siteSearchRunState) markCandidateAvailable(candidate siteSearchCandidate) {
	addSiteSearchCandidateAvailability(candidate, &state.Availability)
}

func (state *siteSearchRunState) markCandidateSeen(guid string) {
	if strings.TrimSpace(guid) == "" {
		return
	}
	state.Seen = append(state.Seen, guid)
	if state.SeenSet != nil {
		state.SeenSet[guid] = struct{}{}
	}
}

func (s *SubscriptionService) logSiteSearchCandidateSkipped(sub *model.Subscription, state *siteSearchRunState, candidate siteSearchCandidate, reason, mediaType, mediaCategory, savePath string) {
	if s.log == nil {
		return
	}
	fields := subscriptionSiteSearchLogFields(sub, state.Keyword)
	fields = append(fields, zap.String("reason", reason))
	fields = appendSiteSearchCandidateLogFields(fields, candidate)
	fields = append(fields, zap.String("media_type", mediaType))
	if mediaCategory != "" {
		fields = append(fields, zap.String("media_category", mediaCategory))
	}
	if savePath != "" {
		fields = append(fields, zap.String("save_path", savePath))
	}
	s.log.Info("site-search subscription candidate skipped", fields...)
}

func (s *SubscriptionService) logSiteSearchCandidateQueued(sub *model.Subscription, state *siteSearchRunState, candidate siteSearchCandidate, mediaType, mediaCategory, savePath string) {
	if s.log == nil {
		return
	}
	fields := subscriptionSiteSearchLogFields(sub, state.Keyword)
	fields = appendSiteSearchCandidateLogFields(fields, candidate)
	fields = append(fields,
		zap.Int("score", candidate.Score),
		zap.String("media_type", mediaType),
		zap.String("media_category", mediaCategory),
		zap.String("save_path", savePath),
	)
	s.log.Info("site-search subscription candidate queued", fields...)
}

func (s *SubscriptionService) logSiteSearchEnqueueFailed(sub *model.Subscription, state *siteSearchRunState, candidate siteSearchCandidate, mediaType, mediaCategory, savePath string, err error) {
	if s.log == nil {
		return
	}
	fields := subscriptionSiteSearchLogFields(sub, state.Keyword)
	fields = appendSiteSearchCandidateLogFields(fields, candidate)
	fields = append(fields,
		zap.String("media_type", mediaType),
		zap.String("media_category", mediaCategory),
		zap.String("save_path", savePath),
		zap.Error(err),
	)
	s.log.Warn("site-search subscription enqueue failed", fields...)
}

func appendSiteSearchCandidateLogFields(fields []zap.Field, candidate siteSearchCandidate) []zap.Field {
	item := candidate.Item
	return append(fields,
		zap.String("title", item.Title),
		zap.String("subtitle", item.Subtitle),
		zap.String("site", firstNonEmpty(item.SiteName, item.SiteID)),
		zap.String("site_category", item.Category),
		zap.Int("season", candidate.Season),
		zap.Int("episode", candidate.Episode),
		zap.Bool("pack", candidate.Pack),
	)
}
