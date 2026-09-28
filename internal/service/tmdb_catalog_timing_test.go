package service

import (
	"testing"
	"time"
)

func TestTMDbCatalogSeasonCooldown(t *testing.T) {
	now := time.Date(2026, 9, 28, 23, 59, 0, 0, time.FixedZone("HK", 8*3600))
	for _, tc := range []struct {
		name, release string
		dates         []string
		days          int
	}{
		{"unknown", "", nil, 3},
		{"malformed", "yesterday", []string{"bad"}, 3},
		{"upcoming", "2025-01-01", []string{"2026-10-28"}, 1},
		{"recent", "2020-01-01", []string{"2026-08-29"}, 1},
		{"within year", "2020-01-01", []string{"2026-08-28"}, 10},
		{"one year boundary", "2025-09-28", nil, 10},
		{"older", "2025-09-27", nil, 20},
		{"distant future", "2027-01-01", nil, 3},
		{"fallback release", "2026-09-28", nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := TMDbSeasonCooldown(tc.release, tc.dates, now); got != time.Duration(tc.days)*24*time.Hour {
				t.Fatalf("got %v want %d days", got, tc.days)
			}
		})
	}
}
