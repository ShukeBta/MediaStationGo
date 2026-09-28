package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestDiscoverFeedUsesServerCacheUnlessRefreshRequested(t *testing.T) {
	gin.SetMode(gin.TestMode)
	discover := service.NewDiscoverService(zap.NewNop(), nil)
	discover.RememberSection("tmdb_latest_movie", 1, []service.ExternalMediaResult{{Title: "cached movie"}})
	svc := &service.Container{Discover: discover}
	router := gin.New()
	router.GET("/discover/feed", discoverFeedHandler(svc))

	request := httptest.NewRequest(http.MethodGet, "/discover/feed?sections=tmdb_latest_movie&page=1", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("cached feed status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var cachedPayload struct {
		Items []service.ExternalMediaResult `json:"tmdb_latest_movie"`
		Meta  map[string]struct {
			Cached bool `json:"cached"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &cachedPayload); err != nil {
		t.Fatal(err)
	}
	if len(cachedPayload.Items) != 1 || cachedPayload.Items[0].Title != "cached movie" {
		t.Fatalf("cached items = %#v", cachedPayload.Items)
	}
	if !cachedPayload.Meta["tmdb_latest_movie"].Cached {
		t.Fatalf("cached meta = %#v", cachedPayload.Meta)
	}

	request = httptest.NewRequest(http.MethodGet, "/discover/feed?sections=tmdb_latest_movie&page=1&refresh=true", nil)
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("refresh feed status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var refreshedPayload struct {
		Items []service.ExternalMediaResult `json:"tmdb_latest_movie"`
		Meta  map[string]struct {
			Cached bool `json:"cached"`
		} `json:"_meta"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &refreshedPayload); err != nil {
		t.Fatal(err)
	}
	if len(refreshedPayload.Items) != 0 {
		t.Fatalf("refresh should bypass cached items, got %#v", refreshedPayload.Items)
	}
	if refreshedPayload.Meta["tmdb_latest_movie"].Cached {
		t.Fatalf("refresh should not report a cache hit: %#v", refreshedPayload.Meta)
	}
}

func TestDiscoverItemDetailRejectsInvalidDoubanID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/discover/items/:source/:provider_id", discoverItemDetailHandler(&service.Container{
		Discover: service.NewDiscoverService(zap.NewNop(), nil),
	}))

	request := httptest.NewRequest(http.MethodGet, "/discover/items/douban/not-a-number?media_type=movie", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "豆瓣 ID 无效") {
		t.Fatalf("invalid douban id response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestTemporarilyDisabledAdultSections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/discover/feed", discoverFeedHandler(&service.Container{}))

	const disabledSections = "adult_followed,adult_javdb_performers_new,adult_javdb_performers_monthly,adult_javdb_performers_fanza"
	request := httptest.NewRequest(http.MethodGet, "/discover/feed?sections="+disabledSections+"&page=2&refresh=true", nil)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("disabled feed status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Meta map[string]struct {
			Disabled bool `json:"disabled"`
			HasNext  bool `json:"has_next"`
		} `json:"_meta"`
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw["_meta"], &payload.Meta); err != nil {
		t.Fatal(err)
	}
	for _, key := range strings.Split(disabledSections, ",") {
		var items []service.ExternalMediaResult
		if err := json.Unmarshal(raw[key], &items); err != nil {
			t.Fatal(err)
		}
		if len(items) != 0 || !payload.Meta[key].Disabled || payload.Meta[key].HasNext {
			t.Fatalf("disabled payload for %s = %#v meta=%#v", key, items, payload.Meta[key])
		}
	}
}

func TestTemporarilyDisabledAdultSectionsAreHiddenFromCatalog(t *testing.T) {
	for _, key := range []string{"adult_followed", "adult_javdb_performers_new", "adult_javdb_performers_monthly", "adult_javdb_performers_fanza"} {
		if !discoverSectionTemporarilyDisabled(key) {
			t.Fatalf("%s must be hidden while temporarily disabled", key)
		}
	}
}

func TestDiscoverWorkPageUsesEighteenVisibleItemsAndOneNextProbe(t *testing.T) {
	items := make([]service.ExternalMediaResult, discoverWorkPageSize+1)
	for index := range items {
		items[index].Title = strconv.Itoa(index + 1)
	}
	visible := discoverSectionVisibleItems("tmdb_latest_movie", items)
	if len(visible) != discoverWorkPageSize {
		t.Fatalf("visible items = %d, want %d", len(visible), discoverWorkPageSize)
	}
	if !discoverSectionHasNext("tmdb_latest_movie", len(items)) {
		t.Fatal("19th item should enable the next page")
	}
	if discoverSectionHasNext("tmdb_latest_movie", discoverWorkPageSize) {
		t.Fatal("exactly 18 items should not enable the next page")
	}
}

func TestDiscoverPerformerSectionsKeepTheirOwnPageSize(t *testing.T) {
	items := make([]service.ExternalMediaResult, 30)
	visible := discoverSectionVisibleItems("adult_javdb_performers_monthly", items)
	if len(visible) != len(items) {
		t.Fatalf("performer items = %d, want %d", len(visible), len(items))
	}
	if discoverSectionHasNext("adult_javdb_performers_monthly", len(items)) {
		t.Fatal("performer list should keep its existing non-paged behavior")
	}
}

func TestDiscoverStaticSectionWindowKeepsContinuousItems(t *testing.T) {
	items := make([]service.ExternalMediaResult, 30)
	for index := range items {
		items[index].Title = strconv.Itoa(index + 1)
	}
	window := discoverSectionWindow(items, 2)
	if len(window) != 12 || window[0].Title != "19" || window[11].Title != "30" {
		t.Fatalf("page 2 window = %#v", window)
	}
}

func TestRememberDiscoverStaticWindowsPreloadsFollowingPages(t *testing.T) {
	discover := service.NewDiscoverService(zap.NewNop(), nil)
	svc := &service.Container{Discover: discover}
	items := make([]service.ExternalMediaResult, 49)
	rememberDiscoverStaticWindows(svc, "adult_javdb_popular", items)

	page1, ok := discover.CachedSection("adult_javdb_popular", 1)
	if !ok || len(page1) != discoverWorkPageSize+1 {
		t.Fatalf("page 1 cache = %d, ok=%v", len(page1), ok)
	}
	page2, ok := discover.CachedSection("adult_javdb_popular", 2)
	if !ok || len(page2) != discoverWorkPageSize+1 {
		t.Fatalf("page 2 cache = %d, ok=%v", len(page2), ok)
	}
	page3, ok := discover.CachedSection("adult_javdb_popular", 3)
	if !ok || len(page3) != 13 {
		t.Fatalf("page 3 cache = %d, ok=%v", len(page3), ok)
	}
}

func TestDiscoverProviderEnabledHonorsAPIConfigToggle(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.APIConfig{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	apiConfig := service.NewAPIConfigService(zap.NewNop(), repos, service.NewCryptoService("", zap.NewNop()))
	enabled := false
	if _, err := apiConfig.Update(t.Context(), "douban", service.APIConfigPatch{Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{APIConfig: apiConfig}

	if discoverProviderEnabled(t.Context(), svc, "douban") {
		t.Fatal("disabled API config should disable discover provider")
	}
	if !discoverProviderEnabled(t.Context(), svc, "missing-provider") {
		t.Fatal("missing API config should keep discover provider available")
	}
}

func TestDiscoverFetchFailureLogIncludesDiagnostics(t *testing.T) {
	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	logDiscoverFetchFailed(
		&service.Container{Log: logger},
		"tmdb_latest_movie",
		2,
		1500*time.Millisecond,
		discoverSectionTimeout("tmdb_latest_movie"),
		context.DeadlineExceeded,
	)

	entries := observed.FilterMessage("discover section fetch failed").All()
	if len(entries) != 1 {
		t.Fatalf("expected one failure log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["section"] != "tmdb_latest_movie" || fields["provider"] != "tmdb" {
		t.Fatalf("unexpected section/provider fields: %#v", fields)
	}
	if fields["page"] != int64(2) && fields["page"] != 2 {
		t.Fatalf("page field missing or wrong: %#v", fields["page"])
	}
	if fields["duration_ms"] != int64(1500) && fields["duration_ms"] != 1500 {
		t.Fatalf("duration_ms field missing or wrong: %#v", fields["duration_ms"])
	}
	if _, ok := fields["timeout"]; !ok {
		t.Fatalf("timeout field missing: %#v", fields)
	}
}

func TestDiscoverSectionTimeoutRaisesBangumiBudget(t *testing.T) {
	if got := discoverSectionTimeout("bangumi_calendar"); got != discoverFeedBangumiTimeout {
		t.Fatalf("bangumi timeout = %s, want %s", got, discoverFeedBangumiTimeout)
	}
	if got := discoverSectionTimeout("tmdb_latest_movie"); got != discoverFeedSectionTimeout {
		t.Fatalf("tmdb timeout = %s, want %s", got, discoverFeedSectionTimeout)
	}
	if got := discoverSectionTimeout("adult_fd2ppv"); got != discoverFeedFD2PPVTimeout {
		t.Fatalf("fd2ppv timeout = %s, want %s", got, discoverFeedFD2PPVTimeout)
	}
}

func TestDiscoverFD2PPVSectionUsesWorkPaging(t *testing.T) {
	if !discoverSectionHasNext("adult_fd2ppv", discoverWorkPageSize+1) {
		t.Fatal("FC2 19th item should enable the next page")
	}
	if got := len(discoverSectionVisibleItems("adult_fd2ppv", make([]service.ExternalMediaResult, discoverWorkPageSize+1))); got != discoverWorkPageSize {
		t.Fatalf("FC2 visible items = %d, want %d", got, discoverWorkPageSize)
	}
}

func TestDiscoverSlowFetchLogIncludesSectionTiming(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	logDiscoverFetchSlow(&service.Container{Log: logger}, "douban_hot_movie", 1, discoverFeedSlowSectionThreshold-time.Millisecond, 24)
	if got := observed.FilterMessage("discover section fetch slow").Len(); got != 0 {
		t.Fatalf("fast section should not log, got %d entries", got)
	}

	logDiscoverFetchSlow(&service.Container{Log: logger}, "douban_hot_movie", 1, discoverFeedSlowSectionThreshold, 24)
	entries := observed.FilterMessage("discover section fetch slow").All()
	if len(entries) != 1 {
		t.Fatalf("expected one slow log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["section"] != "douban_hot_movie" || fields["provider"] != "douban" {
		t.Fatalf("unexpected section/provider fields: %#v", fields)
	}
	if fields["items"] != int64(24) && fields["items"] != 24 {
		t.Fatalf("items field missing or wrong: %#v", fields["items"])
	}
	if _, ok := fields["duration_ms"]; !ok {
		t.Fatalf("duration_ms field missing: %#v", fields)
	}
	if _, ok := fields["slow_threshold"]; !ok {
		t.Fatalf("slow_threshold field missing: %#v", fields)
	}
}

func TestDiscoverFeedErrorMessageHidesTechnicalTimeout(t *testing.T) {
	for _, err := range []error{
		context.DeadlineExceeded,
		errors.New("timeout of 30000ms exceeded"),
	} {
		got := discoverFeedErrorMessage(err)
		if got != "推荐源响应超时，已跳过本次加载" {
			t.Fatalf("message for %q = %q", err, got)
		}
	}
	if got := discoverFeedErrorMessage(errors.New("upstream 503")); got != "推荐源暂时不可用，已跳过本次加载" {
		t.Fatalf("generic message = %q", got)
	}
}

func TestDefaultDiscoverSectionKeysSkipDisabledProviders(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.APIConfig{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	apiConfig := service.NewAPIConfigService(zap.NewNop(), repos, service.NewCryptoService("", zap.NewNop()))
	disabled := false
	for _, provider := range []string{"douban", "bangumi"} {
		if _, err := apiConfig.Update(t.Context(), provider, service.APIConfigPatch{Enabled: &disabled}); err != nil {
			t.Fatal(err)
		}
	}
	svc := &service.Container{APIConfig: apiConfig}

	keys := defaultDiscoverSectionKeys(t.Context(), svc)
	for _, key := range keys {
		switch discoverSectionProvider(key) {
		case "douban", "bangumi":
			t.Fatalf("disabled provider key %q should not be selected by default; keys=%v", key, keys)
		}
	}
	if len(keys) == 0 {
		t.Fatal("default keys should keep enabled providers")
	}
}

func TestDefaultDiscoverSectionKeysIncludeLatestTMDbRails(t *testing.T) {
	keys := defaultDiscoverSectionKeys(t.Context(), &service.Container{})
	keySet := map[string]struct{}{}
	for _, key := range keys {
		keySet[key] = struct{}{}
	}
	for _, key := range []string{"tmdb_latest_movie", "tmdb_latest_tv"} {
		if _, ok := keySet[key]; !ok {
			t.Fatalf("default discover keys should include %q: %v", key, keys)
		}
	}
}

func TestFallbackDiscoverSectionKeyUsesTMDbForDoubanRails(t *testing.T) {
	cases := map[string]string{
		"douban_hot_movie": "tmdb_popular_movie",
		"douban_hot_tv":    "tmdb_popular_tv",
		"douban_top_movie": "tmdb_top_rated_movie",
		"tmdb_latest_tv":   "",
	}
	for key, want := range cases {
		if got := fallbackDiscoverSectionKey(key); got != want {
			t.Fatalf("fallbackDiscoverSectionKey(%q) = %q, want %q", key, got, want)
		}
	}
}
