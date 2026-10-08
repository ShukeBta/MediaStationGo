package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

var chineseReleaseSectionKeys = []string{
	"tmdb_chinese_latest_movie", "tmdb_chinese_latest_tv",
	"tmdb_chinese_upcoming_movie", "tmdb_chinese_upcoming_tv",
}

func TestChineseReleaseFeedsNeverFallBackToPopular(t *testing.T) {
	gin.SetMode(gin.TestMode)
	discover := service.NewDiscoverService(zap.NewNop(), nil)
	for _, key := range []string{"tmdb_popular_movie", "tmdb_popular_tv", "tmdb_chinese_movie", "tmdb_chinese_tv"} {
		discover.RememberSection(key, 1, []service.ExternalMediaResult{{Title: "热门老片"}})
	}
	router := gin.New()
	router.GET("/feed", discoverFeedHandler(&service.Container{Discover: discover}))
	for _, key := range chineseReleaseSectionKeys {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/feed?sections="+key, nil))
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		var items []service.ExternalMediaResult
		var meta map[string]struct{ Error, Fallback string }
		_ = json.Unmarshal(payload[key], &items)
		_ = json.Unmarshal(payload["_meta"], &meta)
		if recorder.Code != http.StatusOK || len(items) != 0 || meta[key].Error == "" || meta[key].Fallback != "" || fallbackDiscoverSectionKey(key) != "" {
			t.Fatalf("%s returned misleading fallback: %s", key, recorder.Body.String())
		}
	}
}

func TestChineseUpcomingFeedFiltersOldCacheWithoutConsumingPageProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now().UTC()
	discover := service.NewDiscoverService(zap.NewNop(), nil)
	for _, key := range []string{"tmdb_chinese_upcoming_movie", "tmdb_chinese_upcoming_tv"} {
		items := make([]service.ExternalMediaResult, discoverWorkPageSize+1)
		for i := range items {
			items[i] = service.ExternalMediaResult{TMDbID: i + 1, Title: "待映", ReleaseDate: now.AddDate(0, 0, 2).Format(time.DateOnly), Countries: []string{"CN"}}
		}
		items[0].ReleaseDate = now.Format(time.DateOnly)
		items[1].ReleaseDate = ""
		items[2].Countries = []string{"US"}
		items[3].ReleaseDate = now.AddDate(0, 0, 1).Format(time.DateOnly)
		discover.RememberSection(key, 1, items)
		router := gin.New()
		router.GET("/feed", discoverFeedHandler(&service.Container{Discover: discover}))
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/feed?sections="+key+"&refresh=true", nil))
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		var visible []service.ExternalMediaResult
		var meta map[string]struct {
			HasNext bool   `json:"has_next"`
			Stale   bool   `json:"stale"`
			Warning string `json:"warning"`
		}
		_ = json.Unmarshal(payload[key], &visible)
		_ = json.Unmarshal(payload["_meta"], &meta)
		if len(visible) != 15 || visible[0].TMDbID != 4 || visible[len(visible)-1].TMDbID != 18 || !meta[key].HasNext || !meta[key].Stale || meta[key].Warning == "" {
			t.Fatalf("stale %s feed violated dates/paging: %s", key, recorder.Body.String())
		}
	}
}

func TestChineseReleaseSectionsHonorTMDbToggleAndDeferPreferenceMigration(t *testing.T) {
	svc, newRouter := newDiscoverPreferenceTestService(t)
	svc.APIConfig = service.NewAPIConfigService(zap.NewNop(), svc.Repo, service.NewCryptoService("", zap.NewNop()))
	disabled := false
	if _, err := svc.APIConfig.Update(t.Context(), "tmdb", service.APIConfigPatch{Enabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	legacy := &model.UserDiscoverPreference{UserID: "legacy", SelectedSections: []string{"tmdb_chinese_movie", "douban_hot_tv"}}
	if err := svc.Repo.DiscoverPreference.Upsert(t.Context(), legacy); err != nil {
		t.Fatal(err)
	}
	got := getDiscoverPreference(t, newRouter("legacy"))
	if got.SectionsVersion != 0 || !slices.Equal(got.SelectedSections, []string{"douban_hot_tv"}) {
		t.Fatalf("disabled TMDb triggered migration: %+v", got)
	}
	for _, section := range enabledDiscoverSections(t.Context(), svc) {
		if slices.Contains(chineseReleaseSectionKeys, section.Key) {
			t.Fatalf("disabled TMDb still exposed %s", section.Key)
		}
	}
	for _, key := range chineseReleaseSectionKeys {
		if discoverSectionProvider(key) != "tmdb" || !discoverSectionUsesWorkPaging(key) {
			t.Fatalf("%s missing provider or pagination registration", key)
		}
	}
	enabled := true
	if _, err := svc.APIConfig.Update(t.Context(), "tmdb", service.APIConfigPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	got = getDiscoverPreference(t, newRouter("legacy"))
	want := []string{"tmdb_chinese_movie", "tmdb_chinese_latest_movie", "tmdb_chinese_upcoming_movie", "douban_hot_tv"}
	if got.SectionsVersion != model.DiscoverSectionsVersion || !slices.Equal(got.SelectedSections, want) {
		t.Fatalf("enabling TMDb lost pending migration: %+v", got)
	}
}
