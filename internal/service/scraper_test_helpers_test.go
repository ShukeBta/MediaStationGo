package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func firstIndexFunc(values []string, match func(string) bool) int {
	for i, value := range values {
		if match(value) {
			return i
		}
	}
	return -1
}

func newTestScraper(t *testing.T) (*ScraperService, *repository.Container, func()) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/tv"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":             12345,
					"name":           "间谍过家家",
					"original_name":  "SPY×FAMILY",
					"overview":       "测试简介",
					"poster_path":    "/poster.jpg",
					"backdrop_path":  "/backdrop.jpg",
					"first_air_date": "2022-04-09",
					"vote_average":   8.6,
				}},
			})
		case r.URL.Path == "/tv/12345/season/2/episode/1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name":         "任务代号: 猫",
				"overview":     "单集剧情",
				"still_path":   "/still.jpg",
				"air_date":     "2023-10-07",
				"vote_average": 9.1,
				"runtime":      24,
			})
		case strings.HasPrefix(r.URL.Path, "/tv/12345"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id":             12345,
				"name":           "间谍过家家",
				"overview":       "测试简介",
				"poster_path":    "/poster.jpg",
				"backdrop_path":  "/backdrop.jpg",
				"first_air_date": "2022-04-09",
				"vote_average":   8.6,
				"origin_country": []string{"JP"},
				"spoken_languages": []map[string]any{{
					"iso_639_1": "ja",
				}},
				"genres": []map[string]any{{
					"name": "Animation",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		upstream.Close()
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		upstream.Close()
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	cfg.Secrets.TMDbImageProxy = upstream.URL + "/images"
	log := zap.NewNop()
	tmdb := NewTMDbProvider(cfg, log, nil)
	scraper := NewScraperService(cfg, log, repos, tmdb, nil, nil, nil, NewHub(log))

	return scraper, repos, upstream.Close
}

func firstQuery(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func lastIndexFunc(values []string, match func(string) bool) int {
	for i := len(values) - 1; i >= 0; i-- {
		if match(values[i]) {
			return i
		}
	}
	return -1
}
