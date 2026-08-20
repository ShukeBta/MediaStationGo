package service

import (
	"context"
	"time"

	"go.uber.org/zap"
)

func (s *ScannerService) invalidateMediaCache(ctx context.Context) {
	if s != nil && s.cache != nil {
		s.cache.DeletePrefix(ctx, "media:")
		s.cache.DeletePrefix(ctx, "stats:")
	}
}

func (s *ScannerService) startAutoScrape(ctx context.Context, libraryID string) {
	scrapeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Minute)
	go func() {
		defer cancel()
		_, err := s.scraper.EnrichLibraryDetailedWithOptions(scrapeCtx, libraryID, skipEpisodeArtworkOptions(false))
		if err != nil {
			s.log.Warn("scraper enrich failed", zap.Error(err))
			return
		}
	}()
}
