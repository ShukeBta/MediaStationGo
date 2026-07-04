package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"
)

func (s *ScraperService) prepareScrapedArtworkURL(ctx context.Context, mediaID, field, current, candidate string) (string, string) {
	current = strings.TrimSpace(current)
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return current, ""
	}
	if candidate == current {
		return candidate, ""
	}
	if s == nil || s.images == nil || !isHTTPish(candidate) {
		return candidate, ""
	}
	fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	err := s.images.PrefetchRemote(fetchCtx, candidate)
	cancel()
	if err != nil {
		if current != "" {
			s.log.Warn("scrape artwork prefetch failed; keeping existing artwork",
				zap.String("media_id", mediaID),
				zap.String("field", field),
				zap.String("candidate", candidate),
				zap.String("existing", current),
				zap.Error(err))
			return current, ""
		}
		s.log.Warn("scrape artwork prefetch failed; keeping new artwork URL for retry",
			zap.String("media_id", mediaID),
			zap.String("field", field),
			zap.String("candidate", candidate),
			zap.Error(err))
		return candidate, ""
	}
	if current != "" && isHTTPish(current) {
		return candidate, current
	}
	return candidate, ""
}

func (s *ScraperService) removeCachedScrapedArtwork(urls ...string) {
	if s == nil || s.images == nil {
		return
	}
	seen := map[string]struct{}{}
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		if err := s.images.RemoveCached(raw); err != nil {
			s.log.Debug("remove old scraped artwork cache failed", zap.String("url", raw), zap.Error(err))
		}
	}
}
