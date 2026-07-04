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

func TestPrepareSubscriptionForRunFillsSeriesMetadata(t *testing.T) {
	var searchedTV bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			searchedTV = true
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":                12345,
					"name":              "南部档案",
					"original_name":     "Archives The Nanyang Mystery",
					"original_language": "zh",
					"origin_country":    []string{"CN"},
					"first_air_date":    "2026-01-01",
				}},
			})
		case "/tv/12345":
			_ = json.NewEncoder(w).Encode(map[string]any{"number_of_episodes": 33})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	repos := repository.New(newServiceTestDB(t, &model.Subscription{}))
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))
	svc := NewSubscriptionService(cfg, zap.NewNop(), repos, nil, nil, nil)
	svc.SetScraper(scraper)

	sub := model.Subscription{Name: "南部档案 自动订阅", FeedURL: "site-search://search?keyword=南部档案", Filter: "南部档案 2026", Enabled: true}
	if err := repos.Subscription.Create(t.Context(), &sub); err != nil {
		t.Fatal(err)
	}

	svc.prepareSubscriptionForRun(t.Context(), &sub)
	if !searchedTV {
		t.Fatal("blank media type subscription should try TV metadata before defaulting to movie")
	}
	if sub.MediaType != "tv" || sub.OriginalName != "Archives The Nanyang Mystery" || sub.Year != 2026 || sub.TotalEpisodes != 33 {
		t.Fatalf("prepared subscription = %#v, want tv metadata with total episodes", sub)
	}

	var stored model.Subscription
	if err := repos.DB.First(&stored, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.MediaType != "tv" || stored.OriginalName != "Archives The Nanyang Mystery" || stored.Year != 2026 || stored.TotalEpisodes != 33 {
		t.Fatalf("stored subscription = %#v, want persisted metadata", stored)
	}
}

func TestSubscriptionMetadataLibraryTypesSearchesTVForBlankType(t *testing.T) {
	got := subscriptionMetadataLibraryTypes("", "南部档案 2026")
	if len(got) < 2 || got[0] != "tv" || got[1] != "movie" {
		t.Fatalf("library types = %#v, want tv before movie for blank subscription type", got)
	}
}
