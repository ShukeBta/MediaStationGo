package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSelectSiteSearchCandidatesRejectsKeywordOriginWithConflictingYear(t *testing.T) {
	sub := &model.Subscription{
		Name:      "玩具总动员 5 自动订阅",
		Filter:    "玩具总动员 5 2026",
		MediaType: "movie",
		Year:      2026,
	}
	results := []SearchResult{{
		Title:         "Toy Story 4 2019 2160p DSNP WEB-DL",
		DownloadURL:   "https://pt/download/toy-story-4",
		SearchKeyword: "玩具总动员 5",
		Seeders:       90,
	}}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, LocalAvailability{})
	if len(got) != 0 {
		t.Fatalf("selected %#v, want conflicting-year keyword-origin result rejected", got)
	}
	if stats.QueryMismatch != 1 || stats.Prepared != 0 {
		t.Fatalf("stats = %#v, want query mismatch for conflicting year", stats)
	}
}

func TestSelectSiteSearchCandidatesDoesNotWashByDefault(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", WashPriority: "resolution"}
	results := []SearchResult{
		{Title: "Inception 2010 1080p", DownloadURL: "https://pt/download/1080", Seeders: 90},
		{Title: "Inception 2010 2160p", DownloadURL: "https://pt/download/2160", Seeders: 80},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/2160" {
		t.Fatalf("selected %#v, want default best single result when wash disabled", got)
	}
}

func TestSelectSiteSearchCandidatesWashNeedsExplicitUpgradeCriteria(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", WashEnabled: true, WashPriority: "resolution"}
	local := LocalAvailability{LocalMediaCount: 1, InLibrary: true}
	results := []SearchResult{
		{Title: "Inception 2010 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/1080", Seeders: 90},
		{Title: "Inception 2010 2160p WEB-DL H264 AAC", DownloadURL: "https://pt/download/2160", Seeders: 80},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, local)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want no wash download without explicit upgrade criteria", got)
	}

	sub.Resolution = "2160p"
	got = selectSiteSearchCandidates(results, sub, map[string]struct{}{}, local)
	if len(got) != 1 || got[0].Download != "https://pt/download/2160" {
		t.Fatalf("selected %#v, want explicit 2160p wash candidate", got)
	}
}

func TestSelectSiteSearchCandidatesWashWithoutCriteriaUsesDefaultQuality(t *testing.T) {
	sub := &model.Subscription{Name: "Dune 自动订阅", Filter: "Dune 2021", MediaType: "movie", WashEnabled: true, WashPriority: "quality"}
	results := []SearchResult{
		{Title: "Dune 2021 2160p REMUX H264 AAC", DownloadURL: "https://pt/download/remux", Seeders: 80},
		{Title: "Dune 2021 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/webdl", Seeders: 60},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/webdl" {
		t.Fatalf("selected %#v, want default compatible WEB-DL when wash has no explicit criteria", got)
	}
}

func TestSelectSiteSearchCandidatesDefaultsToOnePreferredVersionPerEpisode(t *testing.T) {
	sub := &model.Subscription{Name: "House of the Dragon 自动订阅", Filter: "House of the Dragon", MediaType: "tv"}
	results := []SearchResult{
		{Title: "House of the Dragon S03E01 1080p HDTV", DownloadURL: "https://pt/download/e01-hdtv", Seeders: 50000},
		{Title: "House of the Dragon S03E01 1080p WEB-DL", DownloadURL: "https://pt/download/e01-webdl-1080", Seeders: 100},
		{Title: "House of the Dragon S03E01 2160p WEB-DL", DownloadURL: "https://pt/download/e01-webdl-2160", Seeders: 80},
		{Title: "House of the Dragon S03E02 720p HDTV", DownloadURL: "https://pt/download/e02-hdtv", Seeders: 500},
		{Title: "House of the Dragon S03E02 1080p WEBRip", DownloadURL: "https://pt/download/e02-webrip", Seeders: 60},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 2 {
		t.Fatalf("selected %d candidates, want one per episode", len(got))
	}
	if got[0].Download != "https://pt/download/e01-webdl-2160" {
		t.Fatalf("episode 1 selected %q, want best WEB-DL version", got[0].Download)
	}
	if got[1].Download != "https://pt/download/e02-webrip" {
		t.Fatalf("episode 2 selected %q, want WEBRip over high-seeder HDTV", got[1].Download)
	}
}

func TestSelectSiteSearchCandidatesDefaultQualityRecognizesWebDLVariants(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p BluRay H264 AAC", DownloadURL: "https://pt/download/e01-bluray", Seeders: 900},
		{Title: "Some Show S01E01 2160p WEB.DL H264 AAC", DownloadURL: "https://pt/download/e01-webdotdl", Seeders: 40},
		{Title: "Some Show S01E01 1080p WEB DL H264 AAC", DownloadURL: "https://pt/download/e01-webdl", Seeders: 50},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-webdotdl" {
		t.Fatalf("selected %#v, want one best WEB-DL variant", got)
	}
}

func TestSelectSiteSearchCandidatesDefaultPrefersWebDLBeforeResolution(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show S01E01 2160p BluRay H264 AAC", DownloadURL: "https://pt/download/e01-bluray-2160", Seeders: 900},
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-webdl-1080", Seeders: 50},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-webdl-1080" {
		t.Fatalf("selected %#v, want one compatible WEB-DL version before higher-resolution BluRay", got)
	}
}

func TestSelectSiteSearchCandidatesDefaultPrefersFreeWithinSameQualityBand(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-nonfree", Seeders: 5000},
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-free", Seeders: 80, Free: true},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-free" {
		t.Fatalf("selected %#v, want free candidate within same quality/resolution band", got)
	}
}

func TestSelectSiteSearchCandidatesDefaultDoesNotLetFreeOverrideBetterQuality(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p HDTV H264 AAC", DownloadURL: "https://pt/download/e01-free-hdtv", Seeders: 80, Free: true},
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-webdl", Seeders: 50},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-webdl" {
		t.Fatalf("selected %#v, want WEB-DL quality to stay ahead of free HDTV", got)
	}
}

func TestSelectSiteSearchCandidatesDefaultDoesNotLetFreeOverrideBetterResolution(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-free-1080", Seeders: 80, Free: true},
		{Title: "Some Show S01E01 2160p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-2160", Seeders: 50},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-2160" {
		t.Fatalf("selected %#v, want better resolution to stay ahead of free lower-resolution release", got)
	}
}

func TestSelectSiteSearchCandidatesDefaultsToOneLooseNumberedEpisode(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show 01 1080p HDTV H264 AAC", DownloadURL: "https://pt/download/e01-hdtv", Seeders: 5000},
		{Title: "Some Show 01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-webdl-1080", Seeders: 100},
		{Title: "Some Show 01 2160p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-webdl-2160", Seeders: 80},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 {
		t.Fatalf("selected %d candidates, want one preferred loose-numbered episode", len(got))
	}
	if got[0].Download != "https://pt/download/e01-webdl-2160" || got[0].Episode != 1 {
		t.Fatalf("selected %#v, want episode 1 best WEB-DL version", got)
	}
}

func TestSelectSiteSearchCandidatesDoesNotTreatTitleNumberAsLooseEpisode(t *testing.T) {
	sub := &model.Subscription{Name: "问心2 自动订阅", Filter: "问心2 2023", MediaType: "tv"}
	results := []SearchResult{
		{Title: "问心2 2023 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/season", Seeders: 100},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 {
		t.Fatalf("selected %#v, want one fallback candidate", got)
	}
	if got[0].Episode != 0 {
		t.Fatalf("episode = %d, want title number not treated as episode", got[0].Episode)
	}
}

func TestSelectSiteSearchCandidatesRejectsRiskyLabelsFromSiteResult(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", Labels: "HR", DownloadURL: "https://pt/download/e01-hr", Seeders: 900},
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-safe", Seeders: 80},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-safe" {
		t.Fatalf("selected %#v, want non-HR candidate only", got)
	}
}

func TestSelectSiteSearchCandidatesWashPriorityDoesNotLetFreeOverrideResolution(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv", WashEnabled: true, WashPriority: "resolution"}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-free-1080", Seeders: 80, Free: true},
		{Title: "Some Show S01E01 2160p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-2160", Seeders: 50},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-2160" {
		t.Fatalf("selected %#v, want wash resolution priority to stay ahead of free lower-resolution release", got)
	}
}

func TestSelectSiteSearchCandidatesRejectsDefaultCompatibilityVersions(t *testing.T) {
	sub := &model.Subscription{Name: "House of the Dragon 自动订阅", Filter: "House of the Dragon", MediaType: "tv"}
	results := []SearchResult{
		{Title: "House of the Dragon S03E01 2160p WEB-DL HEVC 10bit DoVi Atmos", DownloadURL: "https://pt/download/e01-dovi", Seeders: 900},
		{Title: "House of the Dragon S03E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-webdl", Seeders: 80},
		{Title: "House of the Dragon S03E01 1080p HDTV H264 AAC", DownloadURL: "https://pt/download/e01-hdtv", Seeders: 5000},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-webdl" {
		t.Fatalf("selected %#v, want compatible WEB-DL only", got)
	}
}

func TestSelectSiteSearchCandidatesKeepsCompatibilityExcludesWithCustomExcludeWords(t *testing.T) {
	sub := &model.Subscription{
		Name:         "House of the Dragon 自动订阅",
		Filter:       "House of the Dragon",
		MediaType:    "tv",
		ExcludeWords: "官中,无字幕",
	}
	results := []SearchResult{
		{Title: "House of the Dragon S03E01 2160p WEB-DL HEVC 10bit DoVi Atmos", DownloadURL: "https://pt/download/e01-dovi", Seeders: 900},
		{Title: "House of the Dragon S03E01 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-webdl", Seeders: 80},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/e01-webdl" {
		t.Fatalf("selected %#v, want custom exclude words to keep default compatible WEB-DL guard", got)
	}
}

func TestSelectSiteSearchCandidatesAvoidsOverlappingEpisodeRanges(t *testing.T) {
	sub := &model.Subscription{Name: "House of the Dragon 自动订阅", Filter: "House of the Dragon", MediaType: "tv", WashEnabled: true, WashPriority: "quality"}
	availability := LocalAvailability{
		LocalMediaCount:     1,
		TotalEpisodes:       3,
		MissingEpisodes:     []int{1, 2, 3},
		ExistingEpisodeKeys: map[string]struct{}{},
	}
	results := []SearchResult{
		{Title: "House of the Dragon S03E01-E02 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e01-e02-pack", Seeders: 90},
		{Title: "House of the Dragon S03E02 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e02-duplicate", Seeders: 80},
		{Title: "House of the Dragon S03E03 1080p WEB-DL H264 AAC", DownloadURL: "https://pt/download/e03", Seeders: 70},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 2 {
		t.Fatalf("selected %d candidates, want pack plus non-overlapping episode", len(got))
	}
	if got[0].Download != "https://pt/download/e01-e02-pack" || got[1].Download != "https://pt/download/e03" {
		t.Fatalf("selected %#v, want overlapping E02 duplicate skipped", got)
	}
}

func TestSelectSiteSearchCandidatesAppliesQualityRules(t *testing.T) {
	sub := &model.Subscription{
		Name:         "Dune 自动订阅",
		Filter:       "Dune 2021",
		MediaType:    "movie",
		Resolution:   "2160p",
		Quality:      "remux",
		Effects:      "hdr",
		ExcludeWords: "cam,ts",
	}
	results := []SearchResult{
		{Title: "Dune 2021 2160p WEB-DL HDR", DownloadURL: "https://pt/download/web", Seeders: 100},
		{Title: "Dune 2021 2160p UHD BluRay REMUX HDR", DownloadURL: "https://pt/download/remux", Seeders: 30},
		{Title: "Dune 2021 2160p REMUX HDR CAM", DownloadURL: "https://pt/download/cam", Seeders: 200},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{})
	if len(got) != 1 || got[0].Download != "https://pt/download/remux" {
		t.Fatalf("selected %#v, want filtered remux", got)
	}
}
