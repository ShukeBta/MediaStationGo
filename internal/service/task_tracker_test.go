package service

import "testing"

func TestOrganizeTaskMetricsIncludesScrapeProcessed(t *testing.T) {
	metrics := OrganizeTaskMetrics(&OrganizeResult{
		Scrapes: []OrganizeScrapeSummary{
			{Name: "A", Processed: 3, Matched: 2},
			{Name: "B", Processed: 4, Matched: 1, Error: "failed"},
			{Name: "C", Skipped: true},
		},
	})

	if metrics["scrapes"] != 3 {
		t.Fatalf("scrapes = %d, want 3", metrics["scrapes"])
	}
	if metrics["scrape_processed"] != 7 {
		t.Fatalf("scrape_processed = %d, want 7", metrics["scrape_processed"])
	}
	if metrics["scrape_matched"] != 3 {
		t.Fatalf("scrape_matched = %d, want 3", metrics["scrape_matched"])
	}
	if metrics["scrape_errors"] != 1 {
		t.Fatalf("scrape_errors = %d, want 1", metrics["scrape_errors"])
	}
	if metrics["scrape_skipped"] != 1 {
		t.Fatalf("scrape_skipped = %d, want 1", metrics["scrape_skipped"])
	}
}
