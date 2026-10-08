package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestDiscoverDoubanFallbackKeepsSourceAndWarningAcrossRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	discover := service.NewDiscoverService(zap.NewNop(), nil)
	discover.RememberSection("tmdb_popular_movie", 1, []service.ExternalMediaResult{{Title: "备用电影", Source: "tmdb"}})
	svc := &service.Container{Discover: discover, Douban: service.NewDoubanProvider(&config.Config{}, zap.NewNop())}
	router := gin.New()
	router.GET("/discover/feed", discoverFeedHandler(svc))
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Deterministic provider failure without accessing the real Douban site.

	for attempt := 0; attempt < 2; attempt++ {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/discover/feed?sections=douban_hot_movie", nil).WithContext(ctx)
		router.ServeHTTP(recorder, req)
		var payload struct {
			Items []service.ExternalMediaResult `json:"douban_hot_movie"`
			Meta  map[string]struct {
				Fallback string `json:"fallback"`
				Warning  string `json:"warning"`
				Cached   bool   `json:"cached"`
			} `json:"_meta"`
		}
		if recorder.Code != http.StatusOK {
			t.Fatalf("feed status = %d: %s", recorder.Code, recorder.Body.String())
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		meta := payload.Meta["douban_hot_movie"]
		if len(payload.Items) != 1 || payload.Items[0].Source != "tmdb" || meta.Fallback != "tmdb_popular_movie" || meta.Warning == "" || meta.Cached {
			t.Fatalf("attempt %d lost fallback attribution: %s", attempt, recorder.Body.String())
		}
		if _, ok := discover.CachedSectionLastGood("douban_hot_movie", 1); ok {
			t.Fatal("TMDb fallback must not be saved as successful Douban data")
		}
	}
}

func TestDiscoverRefreshPrefersOwnLastSuccessOverFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	discover := service.NewDiscoverService(zap.NewNop(), nil)
	discover.RememberSection("douban_hot_movie", 1, []service.ExternalMediaResult{{Title: "豆瓣成功结果", Source: "douban"}})
	discover.RememberSection("tmdb_popular_movie", 1, []service.ExternalMediaResult{{Title: "备用电影", Source: "tmdb"}})
	svc := &service.Container{Discover: discover, Douban: service.NewDoubanProvider(&config.Config{}, zap.NewNop())}
	router := gin.New()
	router.GET("/discover/feed", discoverFeedHandler(svc))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/discover/feed?sections=douban_hot_movie&refresh=true", nil).WithContext(ctx)
	router.ServeHTTP(recorder, req)
	var payload struct {
		Items []service.ExternalMediaResult `json:"douban_hot_movie"`
		Meta  map[string]struct {
			Stale    bool   `json:"stale"`
			Warning  string `json:"warning"`
			Fallback string `json:"fallback"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	meta := payload.Meta["douban_hot_movie"]
	if len(payload.Items) != 1 || payload.Items[0].Source != "douban" || !meta.Stale || meta.Warning == "" || meta.Fallback != "" {
		t.Fatalf("expected own successful data: %s", recorder.Body.String())
	}
}

func TestChineseDiscoverSectionsAreSelectableByDefaultAndPaged(t *testing.T) {
	keys := defaultDiscoverSectionKeys(t.Context(), &service.Container{})
	want := []string{
		"tmdb_chinese_movie", "tmdb_chinese_latest_movie", "tmdb_chinese_upcoming_movie",
		"tmdb_chinese_tv", "tmdb_chinese_latest_tv", "tmdb_chinese_upcoming_tv",
		"tmdb_chinese_anime", "tmdb_chinese_variety",
	}
	if len(keys) < len(want) || strings.Join(keys[:len(want)], ",") != strings.Join(want, ",") {
		t.Fatalf("Chinese recommendations should lead the default selection: %v", keys)
	}
	selected, err := normalizeDiscoverPreferenceSections(keys[:len(want)], discoverSectionCatalog, false, true)
	if err != nil || len(selected) != len(want) {
		t.Fatalf("domestic sections must be selectable: %v, %v", selected, err)
	}
	for _, key := range selected {
		items := make([]service.ExternalMediaResult, discoverWorkPageSize+1)
		if discoverSectionProvider(key) != "tmdb" || !discoverSectionHasNext(key, len(items)) || len(discoverSectionVisibleItems(key, items)) != discoverWorkPageSize {
			t.Fatalf("domestic section %s lost provider/pagination", key)
		}
	}
}

func TestChineseDiscoverRailsDispatchToMoviesAnimationAndVariety(t *testing.T) {
	for _, rail := range []struct{ key, label, mediaType, genres string }{
		{"tmdb_chinese_movie", "华语热门电影", "movie", ""},
		{"tmdb_chinese_tv", "华语热门剧集", "tv", ""},
		{"tmdb_chinese_anime", "华语动漫", "tv", "16"},
		{"tmdb_chinese_variety", "华语综艺", "tv", "10764|10767"},
		{"tmdb_chinese_latest_movie", "华语最新电影", "movie", ""},
		{"tmdb_chinese_latest_tv", "华语最新剧集", "tv", ""},
		{"tmdb_chinese_upcoming_movie", "华语待上映电影", "movie", ""},
		{"tmdb_chinese_upcoming_tv", "华语待播剧集", "tv", ""},
	} {
		t.Run(rail.key, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/3/discover/"+rail.mediaType || r.URL.Query().Get("with_genres") != rail.genres || r.URL.Query().Get("with_origin_country") != "CN|HK|TW|MO" {
					t.Errorf("unexpected discovery route: %s %v", r.URL.Path, r.URL.Query())
				}
				if r.URL.Query().Has("without_genres") {
					t.Error("Chinese recommendations must not exclude regular animation or variety genres")
				}
				_, _ = w.Write([]byte(`{"results":[{"id":7,"title":"华语作品","name":"华语作品","genre_ids":[16,10764],"origin_country":["HK"]}]}`))
			}))
			t.Cleanup(server.Close)
			cfg := &config.Config{Secrets: config.SecretsConfig{TMDbAPIKey: "test-key", TMDbAPIProxy: server.URL + "/3/"}}
			svc := &service.Container{Discover: service.NewDiscoverService(zap.NewNop(), service.NewTMDbProvider(cfg, zap.NewNop(), nil))}
			items, err := discoverSectionItems(t.Context(), svc, rail.key, 1, "")
			if err != nil || len(items) != 1 || items[0].MediaType != rail.mediaType {
				t.Fatalf("discovery result = %+v, error = %v", items, err)
			}
			found := false
			for _, section := range discoverSectionCatalog {
				if section.Key == rail.key && section.Label == rail.label {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing Chinese rail label %q", rail.label)
			}
		})
	}
}
