package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestDiscoverTMDbItemDetailUsesSharedServerCache(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/movie/42" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                42,
			"title":             "测试电影",
			"original_title":    "Test Movie",
			"overview":          "完整简介",
			"release_date":      "2026-07-19",
			"runtime":           128,
			"vote_average":      8.2,
			"original_language": "en",
			"production_countries": []map[string]any{{
				"iso_3166_1": "US",
			}},
			"spoken_languages": []map[string]any{{
				"iso_639_1": "en",
			}},
			"genres": []map[string]any{{"name": "剧情"}},
			"credits": map[string]any{
				"cast": []map[string]any{{"id": 7, "name": "演员甲"}},
			},
		})
	}))
	defer upstream.Close()

	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	provider := NewTMDbProvider(cfg, zap.NewNop(), nil)
	discover := NewDiscoverService(zap.NewNop(), provider)

	first, err := discover.TMDbItemDetail(t.Context(), "movie", 42)
	if err != nil {
		t.Fatal(err)
	}
	second, err := discover.TMDbItemDetail(t.Context(), "movie", 42)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatalf("tmdb detail requests = %d, want 1", requests.Load())
	}
	for _, item := range []ExternalMediaResult{first, second} {
		if item.Title != "测试电影" || item.ReleaseDate != "2026-07-19" || item.DurationMinutes != 128 {
			t.Fatalf("detail metadata = %#v", item)
		}
		if len(item.Genres) != 1 || item.Genres[0] != "剧情" || len(item.People) != 1 || item.People[0].Name != "演员甲" {
			t.Fatalf("detail people/genres = %#v", item)
		}
	}
}
