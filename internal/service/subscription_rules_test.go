package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestMatchesSubscriptionRulesUserExcludeWords(t *testing.T) {
	sub := &model.Subscription{ExcludeWords: "10bit,dolby vision,杜比"}
	cases := []struct {
		title string
		want  bool
	}{
		{"Movie 2024 1080p WEB-DL", true},
		{"Movie 2024 2160p 10bit HEVC", false},
		{"Movie 2024 2160p Dolby Vision", false},
		{"电影 2024 杜比全景声", false},
	}
	for _, c := range cases {
		if got := matchesSubscriptionRules(sub, c.title); got != c.want {
			t.Errorf("matchesSubscriptionRules(%q) = %v, want %v", c.title, got, c.want)
		}
	}
}

func TestMatchesSubscriptionRulesReleaseStyleExcludeWords(t *testing.T) {
	cases := []struct {
		name  string
		sub   *model.Subscription
		title string
	}{
		{
			name:  "default excludes ddp channel suffix",
			sub:   &model.Subscription{},
			title: "Some Show 2026 S01E01 1080p WEB-DL DDP5.1 H264",
		},
		{
			name:  "default excludes dolby glued word",
			sub:   &model.Subscription{},
			title: "Some Movie 2026 1080p WEB-DL DolbyVision H264",
		},
		{
			name:  "custom dotted list excludes split tokens",
			sub:   &model.Subscription{ExcludeWords: "DoVi.H265.10bit.杜比"},
			title: "Some Movie 2026 1080p WEB-DL H265",
		},
		{
			name:  "custom dotted list excludes cjk split token",
			sub:   &model.Subscription{ExcludeWords: "DoVi.H265.10bit.杜比"},
			title: "某电影 2026 1080p 杜比全景声",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if matchesSubscriptionRules(c.sub, c.title) {
				t.Fatalf("expected exclude words to reject %q", c.title)
			}
		})
	}
}

func TestMatchesSubscriptionRulesDefaultExcludesJunkReleases(t *testing.T) {
	sub := &model.Subscription{}
	for _, title := range []string{
		"Some Movie 2024 CAM",
		"Some Movie 2024 HDTS",
		"某电影 2024 枪版",
		"Some Movie 2024 TELESYNC",
		"Some Show 预告",
	} {
		if matchesSubscriptionRules(sub, title) {
			t.Errorf("expected default rules to exclude junk release %q", title)
		}
	}
}

func TestMatchesSubscriptionRulesDefaultExcludesRiskyTorrentLabels(t *testing.T) {
	sub := &model.Subscription{}
	for _, title := range []string{
		"Some Show S01E01 1080p WEB-DL HR",
		"Some Show S01E01 1080p WEB-DL H&R",
		"Some Show S01E01 1080p WEB-DL Hit and Run",
		"Some Show S01E01 1080p WEB-DL 禁转",
		"Some Show S01E01 1080p WEB-DL 禁止下载",
	} {
		if matchesSubscriptionRules(sub, title) {
			t.Errorf("expected default rules to exclude risky torrent label %q", title)
		}
	}
}

func TestMatchesSubscriptionRulesDefaultExcludesCompatibilityReleases(t *testing.T) {
	cases := []struct {
		name string
		sub  *model.Subscription
	}{
		{name: "empty exclude words", sub: &model.Subscription{}},
		{name: "legacy frontend defaults", sub: &model.Subscription{ExcludeWords: "cam,ts,tc,枪版"}},
		{name: "custom exclude words", sub: &model.Subscription{ExcludeWords: "官中,无字幕"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for _, title := range []string{
				"Some Movie 2024 2160p DoVi H.265 10bit",
				"Some Movie 2024 2160p H-265",
				"Some Movie 2024 2160p H 265 10 bit",
				"Some Movie 2024 1080p HEVC",
				"Some Movie 2024 1080p x265",
				"Some Movie 2024 2160p Dolby Vision Atmos",
				"某电影 2024 1080p 杜比全景声",
				"Some Anime 2024 1080p Hi10P",
			} {
				if matchesSubscriptionRules(c.sub, title) {
					t.Errorf("expected default compatibility rules to exclude %q", title)
				}
			}
		})
	}
}

func TestMatchesSubscriptionRulesCustomExcludeWordsKeepCompatibilityDefaults(t *testing.T) {
	sub := &model.Subscription{ExcludeWords: "sample"}
	title := "Some Movie 2024 2160p DoVi HEVC 10bit"
	if matchesSubscriptionRules(sub, title) {
		t.Fatalf("custom exclude words should keep default compatibility excludes for %q", title)
	}
	if matchesSubscriptionRules(sub, "Some Movie 2024 1080p SAMPLE") {
		t.Fatal("custom exclude words should still apply")
	}
}

func TestMatchesSubscriptionRulesExplicitEffectsCanRequestCompatibilityFormats(t *testing.T) {
	sub := &model.Subscription{Effects: "dolby vision"}
	title := "Some Movie 2024 2160p DoVi WEB-DL"
	if !matchesSubscriptionRules(sub, title) {
		t.Fatalf("explicit requested effects should allow compatibility format for %q", title)
	}
}

func TestMatchesSubscriptionRulesExplicitAtmosDoesNotAllowOtherCompatibilityFormats(t *testing.T) {
	sub := &model.Subscription{Effects: "atmos"}
	if !matchesSubscriptionRules(sub, "Some Movie 2024 1080p WEB-DL Atmos") {
		t.Fatal("explicit atmos should allow an Atmos-only release")
	}
	if !matchesSubscriptionRules(sub, "Some Movie 2024 1080p WEB-DL Dolby Atmos") {
		t.Fatal("explicit atmos should allow Dolby Atmos wording")
	}
	if matchesSubscriptionRules(sub, "Some Movie 2024 2160p WEB-DL HEVC 10bit DoVi Atmos") {
		t.Fatal("explicit atmos should not also allow DoVi/HEVC/10bit")
	}
}

func TestMatchesSubscriptionRulesWordBoundaryAvoidsFalsePositives(t *testing.T) {
	sub := &model.Subscription{}
	// "ts" / "cam" / "tc" 作为子串出现在合法标题里时不应被默认排除误伤。
	for _, title := range []string{
		"Tsukihime 2024 1080p WEB-DL",
		"Camp Rock 2024 1080p BluRay",
		"Catch Me 2024 1080p WEB-DL",
	} {
		if !matchesSubscriptionRules(sub, title) {
			t.Errorf("word-boundary match wrongly excluded %q", title)
		}
	}
}

func TestSelectSiteSearchCandidatesSkipsExistingMovieWhenNotWashing(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie"}
	results := []SearchResult{
		{Title: "Inception 2010 2160p 10bit Dolby Vision Atmos", DownloadURL: "https://pt/download/dovi", Seeders: 500},
		{Title: "Inception 2010 1080p WEB-DL", DownloadURL: "https://pt/download/web", Seeders: 90},
	}
	availability := LocalAvailability{LocalMediaCount: 1, InLibrary: true, DownloadedEpisodes: 1, TotalEpisodes: 1}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 0 {
		t.Fatalf("selected %#v, want none (movie already in library, wash disabled)", got)
	}
}

func TestSelectSiteSearchCandidatesAllowsMovieWashUpgradeWithExplicitCriteria(t *testing.T) {
	sub := &model.Subscription{Name: "Inception 自动订阅", Filter: "Inception 2010", MediaType: "movie", Resolution: "2160p", WashEnabled: true, WashPriority: "resolution"}
	results := []SearchResult{
		{Title: "Inception 2010 2160p REMUX", DownloadURL: "https://pt/download/2160", Seeders: 80},
		{Title: "Inception 2010 1080p WEB-DL", DownloadURL: "https://pt/download/1080", Seeders: 200},
	}
	availability := LocalAvailability{LocalMediaCount: 1, InLibrary: true, DownloadedEpisodes: 1, TotalEpisodes: 1}

	got := selectSiteSearchCandidates(results, sub, map[string]struct{}{}, availability)
	if len(got) != 1 || got[0].Download != "https://pt/download/2160" {
		t.Fatalf("selected %#v, want 2160p upgrade allowed when washing", got)
	}
}

func TestSubscriptionItemAlreadyAvailable(t *testing.T) {
	movieSub := &model.Subscription{MediaType: "movie"}
	if !subscriptionItemAlreadyAvailable(movieSub, LocalAvailability{LocalMediaCount: 1}, "Inception 2010 2160p") {
		t.Fatal("movie already in library should be reported available")
	}
	if subscriptionItemAlreadyAvailable(movieSub, LocalAvailability{}, "Inception 2010 2160p") {
		t.Fatal("empty library should not be reported available")
	}
	tvSub := &model.Subscription{MediaType: "tv"}
	avail := LocalAvailability{LocalMediaCount: 1, ExistingEpisodeKeys: map[string]struct{}{episodeKey(1, 2): {}}}
	if !subscriptionItemAlreadyAvailable(tvSub, avail, "Show S01E02 1080p") {
		t.Fatal("existing episode should be reported available")
	}
	if subscriptionItemAlreadyAvailable(tvSub, avail, "Show S01E03 1080p") {
		t.Fatal("missing episode should not be reported available")
	}
}
