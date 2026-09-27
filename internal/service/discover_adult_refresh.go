package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

const (
	adultDiscoverPageSize     = 18
	adultDiscoverRefreshLimit = 10 * time.Minute
)

var adultDiscoverRefreshSorts = []string{"release", "views", "likes", "favorites", "comments"}

// RefreshAdultSections refreshes the source-backed adult discovery rails in
// series. A successful source is committed independently so one failed source
// does not discard the other source's last good cache.
func (d *DiscoverService) RefreshAdultSections(ctx context.Context, adult *AdultProvider) error {
	if d == nil || adult == nil {
		return nil
	}

	var errs []error
	if items, err := adult.DiscoverJavDBPopular(ctx); err != nil {
		errs = append(errs, fmt.Errorf("JavDB discover refresh: %w", err))
		if d.log != nil {
			d.log.Warn("adult discover cache refresh failed", zap.String("source", "javdb"), zap.Error(err))
		}
	} else {
		d.rememberAdultDiscoverWindows("adult_javdb_popular", items)
		if d.log != nil {
			d.log.Info("adult discover cache refreshed", zap.String("source", "javdb"), zap.Int("items", len(items)))
		}
	}

	if !adult.FD2PPVEnabled() {
		return errors.Join(errs...)
	}

	for _, sortKey := range adultDiscoverRefreshSorts {
		items, err := adult.DiscoverFD2PPVWindow(ctx, sortKey, 1, adultDiscoverPageSize)
		if err != nil {
			errs = append(errs, fmt.Errorf("FC2 discover refresh (%s): %w", sortKey, err))
			if d.log != nil {
				d.log.Warn("adult discover cache refresh failed", zap.String("source", "fd2ppv"), zap.String("sort", sortKey), zap.Error(err))
			}
			continue
		}
		d.RememberSection("adult_fd2ppv:"+sortKey, 1, items)
		if d.log != nil {
			d.log.Info("adult discover cache refreshed", zap.String("source", "fd2ppv"), zap.String("sort", sortKey), zap.Int("items", len(items)))
		}
	}

	return errors.Join(errs...)
}

func (d *DiscoverService) rememberAdultDiscoverWindows(key string, items []ExternalMediaResult) {
	for page := 1; ; page++ {
		start := (page - 1) * adultDiscoverPageSize
		if start >= len(items) {
			return
		}
		end := start + adultDiscoverPageSize + 1
		if end > len(items) {
			end = len(items)
		}
		d.RememberSection(key, page, items[start:end])
		if end-start <= adultDiscoverPageSize {
			return
		}
	}
}
