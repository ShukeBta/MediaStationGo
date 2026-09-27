package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSchedulerStartRegistersAdultDiscoverRefresh(t *testing.T) {
	scheduler := NewSchedulerService(zap.NewNop(), nil, nil, nil, nil, nil, nil, "")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	scheduler.Start(ctx)
	defer scheduler.Stop()

	for _, status := range scheduler.Status() {
		if status.Name == "adult_discover_refresh" {
			if status.Interval != (24 * time.Hour).String() {
				t.Fatalf("interval = %q, want 24h", status.Interval)
			}
			return
		}
	}
	t.Fatal("adult_discover_refresh was not registered")
}

func TestNextAdultDiscoverRefreshDelayUsesServerDayBoundary(t *testing.T) {
	location := time.FixedZone("server", 8*60*60)
	before := time.Date(2026, time.August, 7, 1, 30, 0, 0, location)
	if got, want := nextAdultDiscoverRefreshDelay(before), 30*time.Minute; got != want {
		t.Fatalf("before 02:00 delay = %s, want %s", got, want)
	}
	after := time.Date(2026, time.August, 7, 3, 0, 0, 0, location)
	if got, want := nextAdultDiscoverRefreshDelay(after), 23*time.Hour; got != want {
		t.Fatalf("after 02:00 delay = %s, want %s", got, want)
	}
}
