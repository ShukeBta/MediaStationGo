package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEnrichOneUsesAlternateLanguageTitleAndKeepsLocalizedMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		switch r.URL.Query().Get("language") {
		case "en-US":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
				{
					"id": 292696, "name": "The First Jasmine", "original_name": "The First Jasmine",
					"first_air_date": "2026-01-01", "origin_country": []string{"CN"},
				},
			}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
				{
					"id": 999001, "name": "First Love", "original_name": "First Love",
					"first_air_date": "2026-01-01", "origin_country": []string{"US"},
				},
				{
					"id": 292696, "name": "莫离", "original_name": "莫离",
					"first_air_date": "2026-01-01", "origin_country": []string{"CN"},
					"poster_path": "/jasmine.jpg",
				},
			}})
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "未分类", Path: `/media/电视剧/未分类`, Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "The First Jasmine",
		Path:         `/media/电视剧/未分类/The.First.Jasmine.S01E01.2026.mkv`,
		Year:         2026,
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	got, err := repos.Media.FindByID(t.Context(), media.ID)
	if err != nil || got == nil {
		t.Fatalf("load media: %v", err)
	}
	if got.TMDbID != 292696 || got.Title != "莫离" || got.ScrapeStatus != "matched" {
		t.Fatalf("matched media=%+v, want localized correct TMDb result", got)
	}
	if got.SeasonNum != 1 || got.EpisodeNum != 1 {
		t.Fatalf("episode markers changed after TV match: season=%d episode=%d", got.SeasonNum, got.EpisodeNum)
	}
}

func TestEnrichOneDoesNotFallbackTVEpisodeToMovie(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
		case "/search/movie":
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
				{"id": 880001, "title": "Unsettled Case", "original_title": "Unsettled Case", "release_date": "2026-01-01"},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "欧美剧", Path: `/media/电视剧/欧美剧`, Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID: lib.ID, Title: "Unsettled Case", Path: `/media/电视剧/欧美剧/Unsettled.Case.S01E04.mkv`,
		SeasonNum: 1, EpisodeNum: 4, ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}
	got, err := repos.Media.FindByID(t.Context(), media.ID)
	if err != nil || got == nil {
		t.Fatalf("load media: %v", err)
	}
	if got.ScrapeStatus != "no_match" || got.TMDbID != 0 {
		t.Fatalf("TV episode incorrectly accepted movie metadata: %+v", got)
	}
	if got.SeasonNum != 1 || got.EpisodeNum != 4 {
		t.Fatalf("TV episode collection state was cleared: season=%d episode=%d", got.SeasonNum, got.EpisodeNum)
	}
}
