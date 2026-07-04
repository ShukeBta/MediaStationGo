package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

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
