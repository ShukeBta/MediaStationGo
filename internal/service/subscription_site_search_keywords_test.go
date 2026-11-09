package service

import (
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSiteSearchKeywordCanUseIMDB(t *testing.T) {
	sub := &model.Subscription{Name: "沙丘 自动订阅", Filter: "Dune 2021", SearchMode: "imdb", IMDBID: "tt1160419"}
	if got := siteSearchKeyword(sub); got != "tt1160419" {
		t.Fatalf("keyword = %q, want imdb id", got)
	}
}

func TestSiteSearchKeywordsIncludeAliasesAndCleanedKeywords(t *testing.T) {
	sub := &model.Subscription{
		Name:    "真人快打2 自动订阅",
		FeedURL: "site-search://search?keyword=%E7%9C%9F%E4%BA%BA%E5%BF%AB%E6%89%932%202026&alias=Mortal%20Kombat%20II%202026",
		Filter:  "真人快打2 2026",
	}

	got := siteSearchKeywords(sub)
	for _, want := range []string{"真人快打2 2026", "Mortal Kombat II 2026", "真人快打2", "Mortal Kombat II"} {
		if !containsString(got, want) {
			t.Fatalf("keywords = %#v, missing %q", got, want)
		}
	}
	if got[0] != "真人快打2 2026" {
		t.Fatalf("primary keyword = %q, want feed keyword first", got[0])
	}
}

func TestSiteSearchKeywordsUseCleanMetadataAliases(t *testing.T) {
	sub := &model.Subscription{
		Name:         "玩具总动员 4 自动订阅",
		Filter:       "玩具总动员 4 2019",
		OriginalName: "Toy Story 4",
		Year:         2019,
	}

	got := siteSearchKeywords(sub)
	for _, want := range []string{"玩具总动员 4 2019", "Toy Story 4", "Toy Story 4 2019", "玩具总动员 4"} {
		if !containsString(got, want) {
			t.Fatalf("keywords = %#v, missing %q", got, want)
		}
	}
	for _, unwanted := range []string{"玩具总动员 4 自动订阅", "玩具总动员 4 自动订阅 2019", "玩具总动员 4 2019 2019"} {
		if containsString(got, unwanted) {
			t.Fatalf("keywords = %#v, should not contain %q", got, unwanted)
		}
	}
}

func TestStableSiteSearchGUIDIgnoresPrivateTokenChanges(t *testing.T) {
	item := SearchResult{
		SiteID:   "mteam",
		Title:    "Some Show S01E01 1080p",
		Category: "TV",
		Size:     1024,
	}
	first := stableSiteSearchGUID(item, "https://pt.example/download?id=123&passkey=old")
	second := stableSiteSearchGUID(item, "https://pt.example/download?id=123&passkey=new")
	if first != second {
		t.Fatalf("stableSiteSearchGUID changed with token: %q != %q", first, second)
	}
	if strings.Contains(first, "passkey") || strings.Contains(first, "old") || strings.Contains(first, "new") {
		t.Fatalf("stableSiteSearchGUID leaked private token: %q", first)
	}
}

func TestSelectSiteSearchCandidatesWithStatsExplainsFiltering(t *testing.T) {
	sub := &model.Subscription{Name: "Stats Show 自动订阅", Filter: "Stats Show", MediaType: "tv"}
	seenItem := SearchResult{Title: "Stats Show S01E02 1080p", DownloadURL: "https://pt/download/seen", Seeders: 50}
	seenGUID := stableSiteSearchGUID(seenItem, seenItem.DownloadURL)
	results := []SearchResult{
		{Title: "Different Show S01E01 1080p", DownloadURL: "https://pt/download/wrong", Seeders: 90},
		{Title: "Stats Show S01E01 CAM", DownloadURL: "https://pt/download/cam", Seeders: 80},
		{Title: "Stats Show S01E02 1080p", Seeders: 70},
		seenItem,
		{Title: "Stats Show S01E03 1080p", DownloadURL: "https://pt/download/right", Seeders: 60},
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{seenGUID: {}}, LocalAvailability{})
	if len(got) != 1 || got[0].Download != "https://pt/download/right" {
		t.Fatalf("selected %#v, want only unfiltered candidate", got)
	}
	if stats.Total != 5 ||
		stats.QueryMismatch != 1 ||
		stats.RuleMismatch != 1 ||
		stats.MissingDownload != 1 ||
		stats.Seen != 1 ||
		stats.Prepared != 1 ||
		stats.Selected != 1 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	if len(stats.QueryMismatchExamples) != 1 || stats.QueryMismatchExamples[0] != "Different Show S01E01 1080p" {
		t.Fatalf("query mismatch examples = %#v", stats.QueryMismatchExamples)
	}
}

func TestKeywordSiteSearchRejectsSubstringDifferentSeries(t *testing.T) {
	sub := &model.Subscription{
		Name:       "谷雨",
		Filter:     "谷雨",
		FeedURL:    "site-search://search?keyword=%E8%B0%B7%E9%9B%A8",
		SearchMode: "keyword",
		MediaType:  "tv",
	}
	results := []SearchResult{{
		Title:       "倾世锦鳞谷雨来 2020 S01 Complete 1080p WEB-DL",
		DownloadURL: "https://pt/download/wrong-series",
	}}

	if got, _ := selectSiteSearchCandidatesWithStats(results, sub, nil, LocalAvailability{
		LocalMediaCount: 1, TotalEpisodes: 1, MissingEpisodes: []int{1},
	}); len(got) != 0 {
		t.Fatalf("selected %#v, want substring-only different series rejected", got)
	}
}

func TestKeywordSiteSearchAcceptsExactSameTitleAndYear(t *testing.T) {
	sub := &model.Subscription{
		Name:       "谷雨",
		Filter:     "谷雨",
		FeedURL:    "site-search://search?keyword=%E8%B0%B7%E9%9B%A8",
		SearchMode: "keyword",
		MediaType:  "tv",
		Year:       2026,
	}
	results := []SearchResult{{
		Title:       "谷雨 2026 S01 Complete 1080p WEB-DL",
		DownloadURL: "https://pt/download/right-series",
	}}

	got := selectSiteSearchCandidates(results, sub, nil)
	if len(got) != 1 || got[0].Download != "https://pt/download/right-series" {
		t.Fatalf("selected %#v, want exact same-title same-year candidate", got)
	}
}

func TestKeywordSiteSearchRejectsConflictingCandidateYear(t *testing.T) {
	sub := &model.Subscription{
		Name:       "谷雨",
		Filter:     "谷雨",
		FeedURL:    "site-search://search?keyword=%E8%B0%B7%E9%9B%A8",
		SearchMode: "keyword",
		MediaType:  "tv",
		Year:       2026,
	}
	results := []SearchResult{{
		Title:       "谷雨 2020 S01 Complete 1080p WEB-DL",
		DownloadURL: "https://pt/download/wrong-year",
	}}

	if got := selectSiteSearchCandidates(results, sub, nil); len(got) != 0 {
		t.Fatalf("selected %#v, want conflicting candidate year rejected", got)
	}
}

func TestKeywordSiteSearchKeepsTMDbIdentityMatchingBehavior(t *testing.T) {
	sub := &model.Subscription{
		Name:       "谷雨",
		Filter:     "谷雨",
		FeedURL:    "site-search://search?keyword=%E8%B0%B7%E9%9B%A8",
		SearchMode: "keyword",
		MediaType:  "tv",
		TMDbID:     123456,
	}
	results := []SearchResult{{
		Title:       "倾世锦鳞谷雨来 2020 S01 Complete 1080p WEB-DL",
		DownloadURL: "https://pt/download/identity-path",
	}}

	if got := selectSiteSearchCandidates(results, sub, nil); len(got) != 1 {
		t.Fatalf("selected %#v, want existing identity-subscription behavior preserved", got)
	}
}

func TestKeywordSiteSearchAcceptsBareEpisodeTitleForms(t *testing.T) {
	sub := &model.Subscription{
		Name:      "Show",
		Filter:    "Show",
		FeedURL:   SiteSearchURL("Show", "", "", false),
		MediaType: "tv",
	}
	for _, title := range []string{"Show 01 1080p", "Show 第1-2集 1080p"} {
		t.Run(title, func(t *testing.T) {
			results := []SearchResult{{Title: title, DownloadURL: "https://pt/download/" + title}}
			if got := selectSiteSearchCandidates(results, sub, nil); len(got) != 1 {
				t.Fatalf("selected %#v, want bare episode title accepted", got)
			}
		})
	}
}

func TestKeywordSiteSearchMatchesPTSeasonFixture(t *testing.T) {
	// Match the PT regression fixture exactly: no Name, SearchMode, or TMDbID.
	sub := &model.Subscription{
		FeedURL:      SiteSearchURL("Show", "", "", false),
		Filter:       "Show",
		MediaType:    "tv",
		SeasonNumber: 2,
	}
	for _, title := range []string{
		"Show 01 1080p",
		"Show01",
		"Show 第1-2集 1080p",
		"Show 第1集 1080p",
		"Show EP01 1080p",
	} {
		t.Run("accept/"+title, func(t *testing.T) {
			results := []SearchResult{{Title: title, DownloadURL: "https://tracker/download"}}
			got := selectSiteSearchCandidates(results, sub, nil)
			if len(got) != 1 || got[0].Season != 2 || got[0].Episode != 1 || len(got[0].Episodes) == 0 || got[0].Episodes[0] != 1 {
				t.Fatalf("selected %#v, want one season 2 episode 1 candidate", got)
			}
		})
	}
	for _, title := range []string{
		"Show S01E01 1080p",
		"Show 第一季第1集",
		"Show 1x01",
		"Show S01 Complete",
	} {
		t.Run("reject/"+title, func(t *testing.T) {
			results := []SearchResult{{Title: title, DownloadURL: "https://tracker/download"}}
			if got := selectSiteSearchCandidates(results, sub, nil); len(got) != 0 {
				t.Fatalf("selected %#v, want explicit other-season candidate rejected", got)
			}
		})
	}
}

func TestKeywordSiteSearchRejectsLongerBareEpisodeTitle(t *testing.T) {
	sub := &model.Subscription{
		Name:      "Show",
		Filter:    "Show",
		FeedURL:   SiteSearchURL("Show", "", "", false),
		MediaType: "tv",
	}
	results := []SearchResult{{
		Title:       "Showtime 01 1080p",
		DownloadURL: "https://pt/download/wrong-series",
	}}

	if got := selectSiteSearchCandidates(results, sub, nil); len(got) != 0 {
		t.Fatalf("selected %#v, want longer bare-episode title rejected", got)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
