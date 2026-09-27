package service

import (
	"context"
	"time"
)

const adultDiscoverRefreshHour = 2

func (s *SchedulerService) jobRefreshAdultDiscover(ctx context.Context) error {
	if s == nil || s.discover == nil || s.adult == nil {
		return nil
	}
	refreshCtx, cancel := context.WithTimeout(ctx, adultDiscoverRefreshLimit)
	defer cancel()
	return s.discover.RefreshAdultSections(refreshCtx, s.adult)
}

func nextAdultDiscoverRefreshDelay(now time.Time) time.Duration {
	if now.IsZero() {
		now = time.Now()
	}
	next := time.Date(now.Year(), now.Month(), now.Day(), adultDiscoverRefreshHour, 0, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(now)
}
