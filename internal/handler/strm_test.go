package handler

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestSTRMRefreshTaskMetricsIncludesScanAndScrape(t *testing.T) {
	metrics := strmRefreshTaskMetrics(
		&service.ScanResult{Visited: 3, Added: 2, Updated: 1, ErrorCount: 1},
		service.EnrichLibraryResult{Matched: 4, Processed: 5, Candidates: 6, Failed: 1},
		2,
	)

	want := map[string]int64{
		"visited":             3,
		"added":               2,
		"updated":             1,
		"errors":              1,
		"scrape_matched":      4,
		"scrape_processed":    5,
		"scrape_candidates":   6,
		"scrape_failed":       1,
		"scrape_reclassified": 2,
	}
	for key, value := range want {
		if metrics[key] != value {
			t.Fatalf("metrics[%q] = %d, want %d in %#v", key, metrics[key], value, metrics)
		}
	}
}

func TestSTRMRefreshScrapeSkipReasonRequiresScrapeRequest(t *testing.T) {
	if got := strmRefreshScrapeSkipReason(&service.STRMRefreshResult{Requested: true, Reason: "no strm changes"}); got != "" {
		t.Fatalf("skip reason without scrape request = %q, want empty", got)
	}

	got := strmRefreshScrapeSkipReason(&service.STRMRefreshResult{
		Requested:       true,
		ScrapeRequested: true,
		Reason:          "no matching local library",
	})
	if got != "refresh not queued: no matching local library" {
		t.Fatalf("skip reason = %q", got)
	}
}
