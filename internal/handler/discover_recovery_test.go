package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if len(keys) < 2 || keys[0] != "tmdb_chinese_movie" || keys[1] != "tmdb_chinese_tv" {
		t.Fatalf("domestic recommendations should lead the default selection: %v", keys)
	}
	selected, err := normalizeDiscoverPreferenceSections(keys[:2], discoverSectionCatalog, false, true)
	if err != nil || len(selected) != 2 {
		t.Fatalf("domestic sections must be selectable: %v, %v", selected, err)
	}
	for _, key := range selected {
		items := make([]service.ExternalMediaResult, discoverWorkPageSize+1)
		if discoverSectionProvider(key) != "tmdb" || !discoverSectionHasNext(key, len(items)) || len(discoverSectionVisibleItems(key, items)) != discoverWorkPageSize {
			t.Fatalf("domestic section %s lost provider/pagination", key)
		}
	}
}
