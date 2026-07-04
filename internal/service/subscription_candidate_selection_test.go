package service

import (
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSelectSiteSearchCandidatesPrefersSeriesPack(t *testing.T) {
	sub := &model.Subscription{Name: "间谍过家家 自动订阅", Filter: "间谍过家家 2022", MediaType: "tv"}
	results := []SearchResult{
		{Title: "间谍过家家 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 80},
		{Title: "间谍过家家 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 50},
		{Title: "间谍过家家 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 70},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 {
		t.Fatalf("selected %d candidates, want 1", len(got))
	}
	if got[0].Download != "https://pt/download/pack" || !got[0].Pack {
		t.Fatalf("selected %#v, want complete pack", got[0])
	}
}

func TestSelectSiteSearchCandidatesQueuesDistinctEpisodesWhenNoPack(t *testing.T) {
	sub := &model.Subscription{Name: "葬送的芙莉莲 自动订阅", Filter: "葬送的芙莉莲", MediaType: "anime", WashEnabled: true, WashPriority: "resolution"}
	results := []SearchResult{
		{Title: "葬送的芙莉莲 S01E01 1080p", DownloadURL: "https://pt/download/1a", Seeders: 90},
		{Title: "葬送的芙莉莲 S01E01 2160p", DownloadURL: "https://pt/download/1b", Seeders: 80},
		{Title: "葬送的芙莉莲 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 70},
		{Title: "葬送的芙莉莲 S01E03 1080p", DownloadURL: "https://pt/download/3", Seeders: 60},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 3 {
		t.Fatalf("selected %d candidates, want 3", len(got))
	}
	if got[0].Episode != 1 || got[1].Episode != 2 || got[2].Episode != 3 {
		t.Fatalf("episodes = %d,%d,%d; want 1,2,3", got[0].Episode, got[1].Episode, got[2].Episode)
	}
	if got[0].Download != "https://pt/download/1b" {
		t.Fatalf("duplicate episode should keep wash-priority best result, got %q", got[0].Download)
	}
}

func TestSelectSiteSearchCandidatesKeepsMovieSingleBest(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", WashPriority: "seeders"}
	results := []SearchResult{
		{Title: "Inception 2010 1080p HDTV", DownloadURL: "https://pt/download/1080-hdtv", Seeders: 900},
		{Title: "Inception 2010 2160p WEB-DL", DownloadURL: "https://pt/download/2160-webdl", Seeders: 80},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/2160-webdl" {
		t.Fatalf("selected %#v, want movie best only", got)
	}
}

func TestSelectSiteSearchCandidatesRejectsUnrelatedHighSeederResult(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", WashPriority: "seeders"}
	results := []SearchResult{
		{Title: "Unrelated Movie 2026 2160p", DownloadURL: "https://pt/download/wrong", Seeders: 999},
		{Title: "Inception 2010 1080p", DownloadURL: "https://pt/download/right", Seeders: 90},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want title-matched result only", got)
	}
}

func TestSelectSiteSearchCandidatesMatchesTranslatedSubtitle(t *testing.T) {
	sub := &model.Subscription{Name: "真人快打2 自动订阅", Filter: "真人快打2 2026", MediaType: "movie", WashPriority: "seeders"}
	results := []SearchResult{
		{Title: "Unrelated Movie 2026 2160p", DownloadURL: "https://pt/download/wrong", Seeders: 999},
		{Title: "Mortal Kombat II 2026 1080p WEB-DL", Subtitle: "真人快打2", DownloadURL: "https://pt/download/right", Seeders: 90},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want translated subtitle match", got)
	}
}

func TestSelectSiteSearchCandidatesMatchesFeedAlias(t *testing.T) {
	sub := &model.Subscription{
		Name:      "真人快打2 自动订阅",
		FeedURL:   "site-search://search?keyword=%E7%9C%9F%E4%BA%BA%E5%BF%AB%E6%89%932%202026&alias=Mortal%20Kombat%20II%202026",
		Filter:    "真人快打2 2026",
		MediaType: "movie",
	}
	results := []SearchResult{
		{Title: "Mortal Kombat II 2026 1080p WEB-DL", DownloadURL: "https://pt/download/right", Seeders: 90},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want alias-matched result", got)
	}
}

func TestSelectSiteSearchCandidatesMatchesSubscriptionOriginalNameAlias(t *testing.T) {
	sub := &model.Subscription{
		Name:         "玩具总动员 5 自动订阅",
		Filter:       "玩具总动员 5 2026",
		OriginalName: "Toy Story 5",
		Year:         2026,
		MediaType:    "movie",
	}
	results := []SearchResult{
		{Title: "Toy Story 5 2026 1080p WEB-DL", DownloadURL: "https://pt/download/right", Seeders: 90},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want original-name alias match", got)
	}
}

func TestSelectSiteSearchCandidatesTrustsMatchedSearchKeyword(t *testing.T) {
	sub := &model.Subscription{
		Name:          "南部档案 自动订阅",
		Filter:        "南部档案",
		MediaType:     "tv",
		TotalEpisodes: 33,
	}
	results := []SearchResult{{
		Title:         "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL",
		DownloadURL:   "https://pt/download/nanyang-29-33",
		SearchKeyword: "南部档案 2026",
		Seeders:       90,
	}}
	availability := LocalAvailability{
		TotalEpisodes: 33,
		ExistingEpisodeKeys: map[string]struct{}{
			episodeKey(1, 1): {},
		},
		MissingEpisodes: []int{2, 3, 4, 5},
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Download != "https://pt/download/nanyang-29-33" || !got[0].Pack {
		t.Fatalf("selected %#v, want English pack matched by Chinese search keyword", got)
	}
	if stats.QueryMismatch != 0 || stats.Prepared != 1 || stats.Selected != 1 {
		t.Fatalf("stats = %#v, want keyword-origin match without query mismatch", stats)
	}
}

func TestDedupeSiteSearchResultsKeepsMatchedSearchKeyword(t *testing.T) {
	sub := &model.Subscription{
		Name:          "南部档案 自动订阅",
		Filter:        "南部档案",
		MediaType:     "tv",
		TotalEpisodes: 33,
	}
	results := dedupeSiteSearchResults([]SearchResult{
		{
			SiteID:        "mteam",
			Title:         "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL",
			DownloadURL:   "https://pt/download/nanyang-29-33",
			SearchKeyword: "Archives The Nanyang Mystery",
			Seeders:       80,
			Size:          1024,
		},
		{
			SiteID:        "mteam",
			Title:         "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL",
			DownloadURL:   "https://pt/download/nanyang-29-33",
			SearchKeyword: "南部档案 2026",
			Seeders:       80,
			Size:          1024,
		},
	})
	if len(results) != 1 {
		t.Fatalf("deduped results = %#v, want one merged result", results)
	}
	if !strings.Contains(results[0].SearchKeyword, "南部档案 2026") {
		t.Fatalf("merged search keyword = %q, missing Chinese keyword", results[0].SearchKeyword)
	}
	availability := LocalAvailability{TotalEpisodes: 33, MissingEpisodes: []int{29, 30, 31, 32, 33}, ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}}}
	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Download != "https://pt/download/nanyang-29-33" {
		t.Fatalf("selected %#v, want merged keyword candidate", got)
	}
	if stats.QueryMismatch != 0 || stats.Prepared != 1 {
		t.Fatalf("stats = %#v, want merged keyword to avoid query mismatch", stats)
	}
}
