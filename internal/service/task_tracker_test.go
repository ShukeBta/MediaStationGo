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

func TestCombineOrganizeItemsBuildsPerItemRows(t *testing.T) {
	res := &OrganizeResult{
		Items: []OrganizePreviewItem{
			{Source: `/downloads/Show.S01E01.mkv`, Target: `/media/Show/Show.S01E01.mkv`, Action: "organize", Title: "Show"},
			{Source: `/downloads/dup.mkv`, Action: "skip", Reason: "duplicate in library"},
			{Source: `/downloads/broken.mp4`, Action: "error", Reason: "unsupported codec"},
		},
		Scans: []OrganizeScanSummary{
			{LibraryID: "lib1", Name: "电影库"},
			{LibraryID: "lib2", Name: "剧集库", Error: "scan failed"},
		},
		Scrapes: []OrganizeScrapeSummary{
			{LibraryID: "lib1", Name: "电影库", Matched: 2},
			{LibraryID: "lib2", Name: "剧集库", Error: "scrape failed"},
		},
	}

	items := combineOrganizeItems(res)
	if len(items) != 7 {
		t.Fatalf("len(items) = %d, want 7", len(items))
	}

	// organize items mapped by source path.
	bySource := map[string]TaskItemRecord{}
	for _, item := range items {
		if item.Kind == ItemKindOrganize {
			bySource[item.Source] = item
		}
	}
	if got := bySource["/downloads/Show.S01E01.mkv"]; got.Status != ItemStatusSucceeded || got.Kind != ItemKindOrganize {
		t.Fatalf("organized item = %#v, want succeeded/organize", got)
	}
	if got := bySource["/downloads/broken.mp4"]; got.Status != ItemStatusFailed || got.Error == "" {
		t.Fatalf("error item = %#v, want failed with error", got)
	}
	if got := bySource["/downloads/dup.mkv"]; got.Status != ItemStatusSucceeded {
		t.Fatalf("skip item = %#v, want succeeded", got)
	}

	// scan + scrape items carry library IDs.
	var scanFailed, scrapeFailed bool
	for _, item := range items {
		if item.Kind == ItemKindScan && item.LibraryID == "lib2" {
			scanFailed = item.Status == ItemStatusFailed
		}
		if item.Kind == ItemKindScrape && item.LibraryID == "lib2" {
			scrapeFailed = item.Status == ItemStatusFailed
		}
	}
	if !scanFailed {
		t.Fatal("scan lib2 should be failed")
	}
	if !scrapeFailed {
		t.Fatal("scrape lib2 should be failed")
	}
}

func TestCombineOrganizeItemsHandlesNoItems(t *testing.T) {
	if got := combineOrganizeItems(nil); got != nil {
		t.Fatalf("combineOrganizeItems(nil) = %#v, want nil", got)
	}
	if got := combineOrganizeItems(&OrganizeResult{}); got != nil {
		t.Fatalf("combineOrganizeItems(empty) = %#v, want nil", got)
	}
}
