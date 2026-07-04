package service

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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
