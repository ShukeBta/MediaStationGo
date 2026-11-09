package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestBackfillSiteSearchTotalEpisodesUsesMetadataOnly(t *testing.T) {
	for _, test := range []struct {
		name             string
		mediaType        string
		noMetadata       bool
		season           int
		total            int
		metadataTotal    int
		seasonEpisodes   []int
		wantTotal        int
		wantLookup       bool
		wantSeasonLookup bool
	}{
		{name: "default_season_total", mediaType: "tv", metadataTotal: 36, seasonEpisodes: []int{1, 2, 3}, wantTotal: 3, wantSeasonLookup: true},
		{name: "known_total_refreshed", mediaType: "tv", total: 8, metadataTotal: 36, seasonEpisodes: []int{1, 2, 3}, wantTotal: 3, wantSeasonLookup: true},
		{name: "season_one_unknown", mediaType: "tv", season: 1, metadataTotal: 36, seasonEpisodes: []int{1, 2, 3}, wantTotal: 3, wantSeasonLookup: true},
		{name: "season_one_without_media_type", season: 1, metadataTotal: 36, seasonEpisodes: []int{1, 2, 3}, wantTotal: 3, wantSeasonLookup: true},
		{name: "season_two_unknown", mediaType: "tv", season: 2, metadataTotal: 36, seasonEpisodes: []int{1, 2}, wantTotal: 2, wantSeasonLookup: true},
		{name: "season_metadata_unavailable", mediaType: "tv", season: 1, metadataTotal: 36, wantSeasonLookup: true},
		{name: "season_metadata_unavailable_keeps_existing", mediaType: "tv", season: 1, total: 8, metadataTotal: 36, wantTotal: 8, wantSeasonLookup: true},
		{name: "season_metadata_incomplete", mediaType: "tv", season: 1, metadataTotal: 36, seasonEpisodes: []int{1, 3}, wantSeasonLookup: true},
		{name: "metadata_unavailable", mediaType: "tv", wantSeasonLookup: true},
		{name: "no_metadata_provider", mediaType: "tv", noMetadata: true},
		{name: "movie", mediaType: "movie", metadataTotal: 13},
		{name: "movie_with_season_is_ignored", mediaType: "movie", season: 1, metadataTotal: 13},
	} {
		t.Run(test.name, func(t *testing.T) {
			var detailRequests int
			var seasonRequests int
			upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/search/tv":
					_, _ = writer.Write([]byte(`{"results":[{"id":42,"name":"Some Show","first_air_date":"2026-01-01"}]}`))
				case "/tv/42":
					detailRequests++
					if test.metadataTotal > 0 {
						_ = json.NewEncoder(writer).Encode(map[string]int{"number_of_episodes": test.metadataTotal})
						return
					}
					http.NotFound(writer, request)
				case "/tv/42/season/1", "/tv/42/season/2":
					seasonRequests++
					if len(test.seasonEpisodes) == 0 {
						http.NotFound(writer, request)
						return
					}
					episodes := make([]map[string]int, 0, len(test.seasonEpisodes))
					for _, episode := range test.seasonEpisodes {
						episodes = append(episodes, map[string]int{"episode_number": episode})
					}
					_ = json.NewEncoder(writer).Encode(map[string]any{"episodes": episodes})
				default:
					http.NotFound(writer, request)
				}
			}))
			defer upstream.Close()
			cfg := &config.Config{}
			cfg.Secrets.TMDbAPIKey = "test"
			cfg.Secrets.TMDbAPIProxy = upstream.URL
			repos := repository.New(newServiceTestDB(t, &model.Subscription{}))
			svc := NewSubscriptionService(cfg, zap.NewNop(), repos, nil, nil, nil)
			tmdb := NewTMDbProvider(cfg, zap.NewNop(), nil)
			if !test.noMetadata {
				svc.SetScraper(NewScraperService(cfg, zap.NewNop(), repos, tmdb, nil, nil, nil, NewHub(zap.NewNop())))
			}
			sub := model.Subscription{
				Name: "Some Show", Filter: "Some Show", FeedURL: "site-search://search?keyword=Some+Show",
				MediaType: test.mediaType, SeasonNumber: test.season, TotalEpisodes: test.total,
			}
			if err := repos.Subscription.Create(t.Context(), &sub); err != nil {
				t.Fatal(err)
			}
			if err := repos.DB.Model(&sub).Update("season_number", test.season).Error; err != nil {
				t.Fatal(err)
			}
			sub.SeasonNumber = test.season
			svc.prepareSubscriptionForRun(t.Context(), &sub)
			svc.backfillSiteSearchTotalEpisodes(t.Context(), &sub)
			if sub.TotalEpisodes != test.wantTotal {
				t.Fatalf("total = %d, want %d", sub.TotalEpisodes, test.wantTotal)
			}
			var stored model.Subscription
			if err := repos.DB.First(&stored, "id = ?", sub.ID).Error; err != nil {
				t.Fatal(err)
			}
			if stored.TotalEpisodes != test.wantTotal {
				t.Fatalf("stored total = %d, want %d", stored.TotalEpisodes, test.wantTotal)
			}
			if (detailRequests > 0) != test.wantLookup {
				t.Fatalf("metadata detail requests = %d, want lookup = %v", detailRequests, test.wantLookup)
			}
			if (seasonRequests > 0) != test.wantSeasonLookup {
				t.Fatalf("season requests = %d, want lookup = %v", seasonRequests, test.wantSeasonLookup)
			}
			if sub.TotalEpisodes > 0 {
				existing := map[string]struct{}{}
				season := subscriptionSeasonNumber(&sub)
				for episode := 1; episode <= sub.TotalEpisodes; episode++ {
					existing[episodeKey(season, episode)] = struct{}{}
				}
				availability := svc.finalizePendingAvailability(&sub, LocalAvailability{
					TotalEpisodes: sub.TotalEpisodes, LocalMediaCount: len(existing), ExistingEpisodeKeys: existing,
					MissingEpisodeKeys: map[string]struct{}{},
				})
				stats := &siteSearchSelectionStats{}
				selected := selectPreparedSubscriptionCandidatesWithStats([]siteSearchCandidate{{Season: sub.SeasonNumber, Episode: sub.TotalEpisodes + 1}}, &sub, availability, stats)
				if len(selected) != 0 || !stats.SeriesComplete {
					t.Fatalf("selection = %+v, stats = %+v; want complete series skipped", selected, stats)
				}
			}
		})
	}
}

func TestRunSiteSearchBackfillsSeasonTotalBeforeTrackerSearch(t *testing.T) {
	var seasonRequests int
	upstream := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/search/tv":
			_, _ = writer.Write([]byte(`{"results":[{"id":42,"name":"Some Show","first_air_date":"2026-01-01"}]}`))
		case "/tv/42/season/1":
			seasonRequests++
			_, _ = writer.Write([]byte(`{"episodes":[{"episode_number":1},{"episode_number":2},{"episode_number":3}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer upstream.Close()
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	repos := repository.New(newServiceTestDB(t, &model.Subscription{}, &model.Site{}))
	if err := repos.Subscription.Create(t.Context(), &model.Subscription{
		Base: model.Base{ID: "sub-show"}, Name: "Some Show", Filter: "Some Show",
		FeedURL: "site-search://search?keyword=Some+Show", MediaType: "tv", SeasonNumber: 1,
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewSubscriptionService(cfg, zap.NewNop(), repos, nil, NewSiteService(zap.NewNop(), repos, ""), nil)
	tmdb := NewTMDbProvider(cfg, zap.NewNop(), nil)
	svc.SetScraper(NewScraperService(cfg, zap.NewNop(), repos, tmdb, nil, nil, nil, NewHub(zap.NewNop())))
	sub := &model.Subscription{
		Base: model.Base{ID: "sub-show"}, Name: "Some Show", Filter: "Some Show",
		FeedURL: "site-search://search?keyword=Some+Show", MediaType: "tv", SeasonNumber: 1,
	}
	for run := 0; run < 2; run++ {
		if _, err := svc.runSiteSearch(t.Context(), sub); err == nil {
			t.Fatal("runSiteSearch error = nil, want missing tracker configuration error")
		}
	}
	if sub.TotalEpisodes != 3 || seasonRequests != 2 {
		t.Fatalf("total episodes = %d, season requests = %d; want 3 and a lookup before each tracker search", sub.TotalEpisodes, seasonRequests)
	}
	var stored model.Subscription
	if err := repos.DB.First(&stored, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.TotalEpisodes != 3 {
		t.Fatalf("stored total episodes = %d, want 3", stored.TotalEpisodes)
	}
}

func TestInferSubscriptionTotalEpisodesFromSearchAndRSS(t *testing.T) {
	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	results := []SearchResult{
		{Title: "Some Show S01E01 1080p"},
		{Title: "Some Show S01E12 1080p"},
		{Title: "Other Show S01E99 1080p"},
	}
	if got := inferSearchTotalEpisodes(results, sub); got != 12 {
		t.Fatalf("search inferred total = %d, want 12", got)
	}
	subtitleResults := []SearchResult{
		{Title: "Smoking Behind the Supermarket with You", Subtitle: "躲在超市后门抽烟的两人 S01E12"},
	}
	subtitleSub := &model.Subscription{Name: "躲在超市后门抽烟的两人 自动订阅", Filter: "躲在超市后门抽烟的两人", MediaType: "tv"}
	if got := inferSearchTotalEpisodes(subtitleResults, subtitleSub); got != 12 {
		t.Fatalf("subtitle search inferred total = %d, want 12", got)
	}
	items := []rssItem{
		{Title: "Some Show S01E02 WEB-DL"},
		{Title: "Some Show S01E10 WEB-DL"},
	}
	if got := inferRSSTotalEpisodes(items, sub, compileFilter("Some Show")); got != 10 {
		t.Fatalf("rss inferred total = %d, want 10", got)
	}
}

func TestResolveSubscriptionTotalEpisodesPrefersTMDbOverTitleFallback(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search/tv":
			_, _ = w.Write([]byte(`{"results":[{"id":42,"name":"Some Show","first_air_date":"2026-01-01"}]}`))
		case "/tv/42":
			_, _ = w.Write([]byte(`{"number_of_episodes":13}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	tmdb := NewTMDbProvider(cfg, zap.NewNop(), nil)
	svc := NewSubscriptionService(cfg, zap.NewNop(), nil, nil, nil, NewHub(zap.NewNop()))
	svc.SetScraper(NewScraperService(cfg, zap.NewNop(), nil, tmdb, nil, nil, nil, NewHub(zap.NewNop())))

	sub := &model.Subscription{Name: "Some Show 自动订阅", Filter: "Some Show", MediaType: "tv"}
	if got := svc.resolveSubscriptionTotalEpisodes(t.Context(), sub, 10); got != 13 {
		t.Fatalf("resolved total = %d, want TMDb total 13", got)
	}
}
