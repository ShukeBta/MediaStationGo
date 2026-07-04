package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSelectSiteSearchCandidatesAppliesSeederSizeAndFreeRules(t *testing.T) {
	sub := &model.Subscription{
		Name:       "Some Show 自动订阅",
		Filter:     "Some Show",
		MediaType:  "tv",
		MinSeeders: 10,
		MaxSeeders: 100,
		MinSizeGB:  1,
		MaxSizeGB:  8,
		FreeOnly:   true,
	}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/low-seed", Seeders: 3, Size: 2 * bytesPerGiB, Free: true},
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/hot", Seeders: 500, Size: 2 * bytesPerGiB, Free: true},
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/small", Seeders: 50, Size: bytesPerGiB / 2, Free: true},
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/nonfree", Seeders: 50, Size: 2 * bytesPerGiB},
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/right", Seeders: 50, Size: 2 * bytesPerGiB, Free: true},
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, nil, LocalAvailability{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want only torrent matching seed/size/free rules", got)
	}
	if stats.RuleMismatch != 4 || stats.Prepared != 1 || stats.Selected != 1 {
		t.Fatalf("stats = %#v, want four rule mismatches and one selected", stats)
	}
}

func TestSubscriptionTorrentRulesRecognizeFreeLabels(t *testing.T) {
	sub := &model.Subscription{FreeOnly: true}
	for _, item := range []SearchResult{
		{Title: "Some Movie 2026 1080p WEB-DL FREE"},
		{Title: "Some Movie 2026 1080p WEB-DL FreeLeech"},
		{Title: "Some Movie 2026 1080p WEB-DL 免费"},
		{Title: "Some Movie 2026 1080p WEB-DL", Free: true},
	} {
		if !matchesSubscriptionTorrentRules(sub, item) {
			t.Fatalf("expected free rule to accept %#v", item)
		}
	}
	if matchesSubscriptionTorrentRules(sub, SearchResult{Title: "Some Movie 2026 1080p WEB-DL"}) {
		t.Fatal("free-only rule accepted non-free result")
	}
}

func TestSubscriptionTorrentRulesRejectUnknownSizeWhenSizeRangeConfigured(t *testing.T) {
	sub := &model.Subscription{MinSizeGB: 1}
	if matchesSubscriptionTorrentRules(sub, SearchResult{Title: "Some Movie 2026 1080p WEB-DL"}) {
		t.Fatal("size range accepted result without size metadata")
	}
}
