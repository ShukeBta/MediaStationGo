package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	discoverArtworkPrefetchLimit       = 384
	discoverArtworkPrefetchConcurrency = 8
	discoverArtworkPrefetchTimeout     = 90 * time.Second
)

func (d *DiscoverService) SetImageProxy(images *ImageProxy) *DiscoverService {
	if d != nil {
		d.images = images
	}
	return d
}

func (d *DiscoverService) WarmMatchArtwork(items []Match) int {
	return d.warmArtworkURLs(matchArtworkURLs(items))
}

func matchArtworkURLs(items []Match) []string {
	urls := make([]string, 0, len(items)*2)
	for _, item := range items {
		urls = append(urls, item.PosterURL)
	}
	for _, item := range items {
		urls = append(urls, item.BackdropURL)
	}
	return urls
}

func (d *DiscoverService) WarmExternalArtwork(items []ExternalMediaResult) int {
	return d.warmArtworkURLs(externalArtworkURLs(items))
}

func externalArtworkURLs(items []ExternalMediaResult) []string {
	urls := make([]string, 0, len(items)*2)
	for _, item := range items {
		urls = append(urls, item.PosterURL)
	}
	for _, item := range items {
		urls = append(urls, item.BackdropURL)
	}
	return urls
}

func (d *DiscoverService) warmArtworkURLs(urls []string) int {
	if d == nil || d.images == nil || len(urls) == 0 {
		return 0
	}
	pending := uniqueDiscoverArtworkURLs(urls, discoverArtworkPrefetchLimit)
	if len(pending) == 0 {
		return 0
	}
	if d.log != nil {
		d.log.Debug("discover artwork prefetch scheduled", zap.Int("count", len(pending)))
	}
	go d.prefetchArtworkURLs(pending)
	return len(pending)
}

func uniqueDiscoverArtworkURLs(urls []string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, min(len(urls), limit))
	for _, raw := range urls {
		raw = strings.TrimSpace(raw)
		if raw == "" || !isHTTPish(raw) {
			continue
		}
		if _, ok := seen[raw]; ok {
			continue
		}
		seen[raw] = struct{}{}
		out = append(out, raw)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (d *DiscoverService) prefetchArtworkURLs(urls []string) {
	ctx, cancel := context.WithTimeout(context.Background(), discoverArtworkPrefetchTimeout)
	defer cancel()

	sem := make(chan struct{}, discoverArtworkPrefetchConcurrency)
	var wg sync.WaitGroup
	for _, raw := range urls {
		select {
		case <-ctx.Done():
			return
		case sem <- struct{}{}:
		}
		wg.Add(1)
		go func(raw string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := d.images.PrefetchRemote(ctx, raw); err != nil && d.log != nil {
				d.log.Debug("discover artwork prefetch failed", zap.String("url", raw), zap.Error(err))
			}
		}(raw)
	}
	wg.Wait()
}
