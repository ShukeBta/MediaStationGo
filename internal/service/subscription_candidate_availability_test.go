package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSelectSiteSearchCandidatesOnlyQueuesMissingLocalEpisodes(t *testing.T) {
	sub := &model.Subscription{Name: "间谍过家家 自动订阅", Filter: "间谍过家家", MediaType: "tv", TotalEpisodes: 3}
	results := []SearchResult{
		{Title: "间谍过家家 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 100},
		{Title: "间谍过家家 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 90},
		{Title: "间谍过家家 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "间谍过家家 S01E03 1080p", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	availability := LocalAvailability{
		TotalEpisodes:       3,
		LocalMediaCount:     2,
		MissingEpisodes:     []int{3},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}, episodeKey(1, 2): {}},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 3 {
		t.Fatalf("selected %#v, want only missing episode 3", got)
	}
}

func TestSelectSiteSearchCandidatesWithUnknownTotalSkipsExistingEpisodes(t *testing.T) {
	sub := &model.Subscription{Name: "葬送的芙莉莲 自动订阅", Filter: "葬送的芙莉莲", MediaType: "anime"}
	results := []SearchResult{
		{Title: "葬送的芙莉莲 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 100},
		{Title: "葬送的芙莉莲 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 90},
		{Title: "葬送的芙莉莲 S01E02 1080p", DownloadURL: "https://pt/download/2", Seeders: 80},
		{Title: "葬送的芙莉莲 S01E03 1080p", DownloadURL: "https://pt/download/3", Seeders: 70},
	}
	availability := LocalAvailability{
		LocalMediaCount:     2,
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}, episodeKey(1, 2): {}},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 3 {
		t.Fatalf("selected %#v, want only not-yet-local episode 3", got)
	}
}

func TestSelectSiteSearchCandidatesSingleExistingEpisodeIsSkipped(t *testing.T) {
	sub := &model.Subscription{Name: "葬送的芙莉莲 自动订阅", Filter: "葬送的芙莉莲", MediaType: "anime", TotalEpisodes: 3}
	results := []SearchResult{
		{Title: "葬送的芙莉莲 S01E01 1080p", DownloadURL: "https://pt/download/1", Seeders: 90},
	}
	availability := LocalAvailability{
		TotalEpisodes:       3,
		LocalMediaCount:     1,
		MissingEpisodes:     []int{2, 3},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want none because E01 already exists", got)
	}
}

func TestSelectSiteSearchCandidatesFullPackUsedAsFallbackWhenLibraryPartiallyExists(t *testing.T) {
	// 本地缺第 3 集,站点只有整季全集包(无单集种)。剧集完结后站点常只挂全集包,
	// 此时必须用全集包兜底补缺集,否则"补全缺失集"永远匹配为空(用户报告的 bug)。
	sub := &model.Subscription{Name: "间谍过家家 自动订阅", Filter: "间谍过家家", MediaType: "tv", TotalEpisodes: 3}
	results := []SearchResult{
		{Title: "间谍过家家 S01 Complete 1080p", DownloadURL: "https://pt/download/pack", Seeders: 100},
	}
	availability := LocalAvailability{
		TotalEpisodes:       3,
		LocalMediaCount:     2,
		MissingEpisodes:     []int{3},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 1): {}, episodeKey(1, 2): {}},
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 {
		t.Fatalf("selected %#v, want the full pack as fallback to cover missing episode 3", got)
	}
	if got[0].Download != "https://pt/download/pack" {
		t.Fatalf("selected %#v, want the Complete pack", got)
	}
}

func TestSelectSiteSearchCandidatesPartialSeriesPackDoesNotSatisfySubscription(t *testing.T) {
	sub := &model.Subscription{Name: "问心2 自动订阅", Filter: "问心2", MediaType: "tv", TotalEpisodes: 33}
	results := []SearchResult{
		{Title: "问心2 S01E07 2160p WEB-DL", DownloadURL: "https://pt/download/7", Seeders: 100},
	}
	availability := LocalAvailability{
		TotalEpisodes:       33,
		LocalMediaCount:     7,
		HasSeriesPack:       true,
		MissingEpisodes:     []int{7},
		ExistingEpisodeKeys: map[string]struct{}{},
	}
	for episode := 1; episode <= 6; episode++ {
		availability.ExistingEpisodeKeys[episodeKey(1, episode)] = struct{}{}
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 7 {
		t.Fatalf("selected %#v, want missing episode 7 despite local pack marker", got)
	}
	if stats.LocalSeriesPackPresent {
		t.Fatalf("LocalSeriesPackPresent = true, want false for partial series availability")
	}
}

func TestSelectSiteSearchCandidatesIgnoresUnderestimatedLocalTotal(t *testing.T) {
	sub := &model.Subscription{Name: "南部档案 自动订阅", Filter: "南部档案", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL", SearchKeyword: "南部档案 2026", DownloadURL: "https://pt/download/29-33", Seeders: 100},
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 6; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	availability := LocalAvailability{
		TotalEpisodes:       1,
		LocalMediaCount:     7,
		HasSeriesPack:       true,
		ExistingEpisodeKeys: existing,
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Download != "https://pt/download/29-33" {
		t.Fatalf("selected %#v, want high-episode candidate despite underestimated local total", got)
	}
	if stats.SeriesComplete || stats.NotMissingEpisodeSkipped != 0 {
		t.Fatalf("stats = %#v, underestimated total must not mark series complete or skip high episodes", stats)
	}
}

func TestSelectSiteSearchCandidatesRangeCanCoverMissingEpisodesAfterExistingStart(t *testing.T) {
	sub := &model.Subscription{Name: "南部档案 自动订阅", Filter: "南部档案", MediaType: "tv", TotalEpisodes: 33}
	results := []SearchResult{
		{Title: "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL", SearchKeyword: "南部档案 2026", DownloadURL: "https://pt/download/29-33", Seeders: 100},
	}
	existing := map[string]struct{}{episodeKey(1, 29): {}}
	availability := LocalAvailability{
		TotalEpisodes:       33,
		LocalMediaCount:     1,
		MissingEpisodes:     []int{30, 31, 32, 33},
		ExistingEpisodeKeys: existing,
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Download != "https://pt/download/29-33" {
		t.Fatalf("selected %#v, want range candidate because it covers E30-E33", got)
	}
	if stats.ExistingEpisodeSkipped != 0 || stats.NotMissingEpisodeSkipped != 0 {
		t.Fatalf("stats = %#v, range covering missing episodes must not be skipped", stats)
	}
}

func TestSelectSiteSearchCandidatesMissingEpisodeCanMatchSubtitleAlias(t *testing.T) {
	sub := &model.Subscription{Name: "躲在超市后门抽烟的两人 自动订阅", Filter: "躲在超市后门抽烟的两人", MediaType: "tv", TotalEpisodes: 12}
	results := []SearchResult{
		{Title: "Smoking Behind the Supermarket with You S01E01 1080p", Subtitle: "躲在超市后门抽烟的两人", DownloadURL: "https://pt/download/1", Seeders: 100},
		{Title: "Smoking Behind the Supermarket with You S01E12 1080p", Subtitle: "躲在超市后门抽烟的两人", DownloadURL: "https://pt/download/12", Seeders: 80},
	}
	existing := map[string]struct{}{}
	for episode := 1; episode <= 11; episode++ {
		existing[episodeKey(1, episode)] = struct{}{}
	}
	availability := LocalAvailability{
		TotalEpisodes:       12,
		LocalMediaCount:     11,
		MissingEpisodes:     []int{12},
		ExistingEpisodeKeys: existing,
	}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 12 || got[0].Download != "https://pt/download/12" {
		t.Fatalf("selected %#v, want subtitle-matched missing episode 12", got)
	}
}

func TestSelectSiteSearchCandidatesRelaxesQueryForExistingSeriesMissingEpisodes(t *testing.T) {
	sub := &model.Subscription{Name: "翘楚 S01E06 自动订阅", Filter: "翘楚 S01E06", MediaType: "tv", TotalEpisodes: 24}
	results := []SearchResult{
		{Title: "Qiao Chu 2026 S01E06 2160p WEB-DL", DownloadURL: "https://pt/download/6", Seeders: 10},
		{Title: "Ashes to Crown 2026 S01E21 2160p WEB-DL", DownloadURL: "https://pt/download/21", Seeders: 8},
		{Title: "Ashes to Crown 2026 S01E99 2160p WEB-DL", DownloadURL: "https://pt/download/99", Seeders: 99},
	}
	availability := LocalAvailability{
		TotalEpisodes:       24,
		LocalMediaCount:     1,
		MissingEpisodes:     []int{21},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 6): {}},
	}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Episode != 21 || got[0].Download != "https://pt/download/21" {
		t.Fatalf("selected %#v, want relaxed alias-like missing episode 21 only", got)
	}
	if stats.QueryMismatch != 3 || stats.RelaxedQueryMatch != 3 || stats.ExistingEpisodeSkipped != 1 || stats.NotMissingEpisodeSkipped != 1 {
		t.Fatalf("unexpected relaxed stats: %#v", stats)
	}
}

func TestAddSiteSearchCandidateAvailabilityTracksRelaxedAliasCandidate(t *testing.T) {
	sub := &model.Subscription{Name: "翘楚 S01E06 自动订阅", Filter: "翘楚 S01E06", MediaType: "tv", TotalEpisodes: 24}
	availability := LocalAvailability{
		TotalEpisodes:       24,
		LocalMediaCount:     1,
		MissingEpisodes:     []int{21},
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 6): {}},
		MissingEpisodeKeys:  map[string]struct{}{episodeKey(1, 21): {}},
	}
	candidate := siteSearchCandidate{
		Item: SearchResult{
			Title:       "Ashes to Crown 2026 S01E21 2160p WEB-DL",
			DownloadURL: "https://pt/download/21",
		},
		Download: "https://pt/download/21",
		GUID:     "site|m-team|ashes-to-crown-21",
		Season:   1,
		Episode:  21,
	}

	addSiteSearchCandidateAvailability(candidate, &availability)
	availability = NewSubscriptionService(nil, nil, nil, nil, nil, nil).finalizePendingAvailability(sub, availability)

	if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, 21)]; !ok {
		t.Fatalf("missing relaxed alias candidate E21 key: %#v", availability.ExistingEpisodeKeys)
	}
	got := selectSiteSearchCandidates([]SearchResult{candidate.Item}, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want relaxed alias candidate skipped after dedup availability update", got)
	}
}

func TestAddSiteSearchCandidateAvailabilityTracksEpisodeRange(t *testing.T) {
	availability := LocalAvailability{
		TotalEpisodes:       33,
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
	candidate := siteSearchCandidate{
		Item: SearchResult{
			Title:       "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL",
			DownloadURL: "https://pt/download/29-33",
		},
		Download: "https://pt/download/29-33",
		GUID:     "site|m-team|nanyang-29-33",
		Season:   1,
		Episode:  29,
		Episodes: []int{29, 30, 31, 32, 33},
		Pack:     true,
	}

	addSiteSearchCandidateAvailability(candidate, &availability)
	for episode := 29; episode <= 33; episode++ {
		if _, ok := availability.ExistingEpisodeKeys[episodeKey(1, episode)]; !ok {
			t.Fatalf("availability missing E%d after range mark: %#v", episode, availability.ExistingEpisodeKeys)
		}
	}
}

func TestCandidateAvailableInAvailabilityRequiresFullRangeCoverage(t *testing.T) {
	sub := &model.Subscription{Name: "南部档案 自动订阅", Filter: "南部档案", MediaType: "tv", TotalEpisodes: 33}
	candidate := siteSearchCandidate{
		Item: SearchResult{
			Title:       "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL",
			DownloadURL: "https://pt/download/29-33",
		},
		Season:   1,
		Episode:  29,
		Episodes: []int{29, 30, 31, 32, 33},
		Pack:     true,
	}
	availability := LocalAvailability{
		TotalEpisodes:       33,
		ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 29): {}, episodeKey(1, 30): {}},
	}

	if candidateAvailableInAvailability(sub, candidate, availability) {
		t.Fatal("partial range availability must not confirm a deduped subscription candidate")
	}
	for episode := 31; episode <= 33; episode++ {
		availability.ExistingEpisodeKeys[episodeKey(1, episode)] = struct{}{}
	}
	if !candidateAvailableInAvailability(sub, candidate, availability) {
		t.Fatal("complete range availability should confirm a deduped subscription candidate")
	}
}

func TestShouldSkipExistingTorrentKeepsSeriesRangeCandidate(t *testing.T) {
	svc := &SubscriptionService{downloads: &DownloadService{}}
	candidate := siteSearchCandidate{
		Item: SearchResult{
			Title:       "Archives The Nanyang Mystery 2026 S01E29-E33 2160p WEB-DL",
			DownloadURL: "https://pt/download/29-33",
		},
		Season:   1,
		Episode:  29,
		Episodes: []int{29, 30, 31, 32, 33},
		Pack:     true,
	}

	if svc.shouldSkipExistingTorrent(t.Context(), "tv", candidate) {
		t.Fatal("series range candidate should not be skipped by global torrent-name precheck")
	}
}

func TestSelectSiteSearchCandidatesDoesNotRelaxQueryForMovies(t *testing.T) {
	sub := &model.Subscription{Name: "玩具总动员 5 自动订阅", Filter: "玩具总动员 5 2026", MediaType: "movie"}
	results := []SearchResult{
		{Title: "Toy Story 4 2019 2160p WEB-DL", DownloadURL: "https://pt/download/wrong", Seeders: 100},
	}
	availability := LocalAvailability{}

	got, stats := selectSiteSearchCandidatesWithStats(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want no relaxed movie match", got)
	}
	if stats.QueryMismatch != 1 || stats.RelaxedQueryMatch != 0 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
}

func TestSelectSiteSearchCandidatesSingleExistingMovieIsSkippedWhenNotWashing(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie"}
	results := []SearchResult{
		{Title: "Inception 2010 1080p WEB-DL", DownloadURL: "https://pt/download/web", Seeders: 90},
	}
	availability := LocalAvailability{LocalMediaCount: 1, InLibrary: true, DownloadedEpisodes: 1, TotalEpisodes: 1}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want none because movie already exists and wash is disabled", got)
	}
}
