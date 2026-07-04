package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestScrapeDelayUsesSettings(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()
	if err := repos.DB.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}

	if got := scraper.scrapeDelay(t.Context()); got < 250*time.Millisecond || got > 500*time.Millisecond {
		t.Fatalf("default scrapeDelay = %s, want 250-500ms", got)
	}

	if err := repos.Setting.Set(t.Context(), "scrape.delay_min_ms", "0"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "scrape.delay_max_ms", "0"); err != nil {
		t.Fatal(err)
	}
	if got := scraper.scrapeDelay(t.Context()); got != 0 {
		t.Fatalf("disabled scrapeDelay = %s, want 0", got)
	}

	if err := repos.Setting.Set(t.Context(), "scrape.delay_min_ms", "800"); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "scrape.delay_max_ms", "200"); err != nil {
		t.Fatal(err)
	}
	if got := scraper.scrapeDelay(t.Context()); got != 800*time.Millisecond {
		t.Fatalf("normalized scrapeDelay = %s, want 800ms", got)
	}
}

func TestApplyProviderMatchInvalidatesMediaCache(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	cache := NewRuntimeCacheService(&config.Config{}, zap.NewNop())
	cache.SetJSON(t.Context(), "media:list:stale", map[string]string{"poster": ""}, time.Minute)
	cache.SetJSON(t.Context(), "stats:snapshot:base", map[string]int{"media": 1}, time.Minute)
	scraper.SetRuntimeCache(cache)

	libPath := t.TempDir()
	lib := model.Library{Name: "Movies", Path: libPath, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Title: "Raw", Path: "/media/movies/raw.mkv", ScrapeStatus: "pending"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	match := &Match{Title: "Matched", PosterURL: "https://image.tmdb.org/t/p/w500/poster.jpg", BackdropURL: "https://image.tmdb.org/t/p/w1280/backdrop.jpg"}
	if err := scraper.applyProviderMatch(t.Context(), &media, &lib, match); err != nil {
		t.Fatal(err)
	}

	var stale map[string]string
	if cache.GetJSON(t.Context(), "media:list:stale", &stale) {
		t.Fatal("scraper should invalidate media list cache after applying artwork")
	}
	var stats map[string]int
	if cache.GetJSON(t.Context(), "stats:snapshot:base", &stats) {
		t.Fatal("scraper should invalidate stats cache after applying artwork")
	}
	var stored model.Media
	if err := repos.DB.First(&stored, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PosterURL == "" || stored.BackdropURL == "" || stored.ScrapeStatus != "matched" {
		t.Fatalf("match not saved: poster=%q backdrop=%q status=%q", stored.PosterURL, stored.BackdropURL, stored.ScrapeStatus)
	}
}

func TestApplyProviderMatchKeepsExistingArtworkWhenNewPrefetchFails(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	images.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("bad gateway")),
			Request:    req,
		}, nil
	})}
	scraper.SetImageProxy(images)

	libPath := t.TempDir()
	lib := model.Library{Name: "Movies", Path: libPath, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	oldPoster := "https://image.tmdb.org/t/p/w500/old-poster.jpg"
	oldBackdrop := "https://image.tmdb.org/t/p/w1280/old-backdrop.jpg"
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Raw",
		Path:         filepath.Join(libPath, "raw.mkv"),
		PosterURL:    oldPoster,
		BackdropURL:  oldBackdrop,
		ScrapeStatus: "matched",
		OriginalName: "Raw",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	match := &Match{
		Title:       "Matched",
		PosterURL:   "https://image.tmdb.org/t/p/w500/new-broken-poster.jpg",
		BackdropURL: "",
	}
	if err := scraper.applyProviderMatch(t.Context(), &media, &lib, match); err != nil {
		t.Fatal(err)
	}

	var stored model.Media
	if err := repos.DB.First(&stored, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PosterURL != oldPoster {
		t.Fatalf("poster should keep existing URL when new prefetch fails: got %q want %q", stored.PosterURL, oldPoster)
	}
	if stored.BackdropURL != oldBackdrop {
		t.Fatalf("blank match backdrop should not clear existing backdrop: got %q want %q", stored.BackdropURL, oldBackdrop)
	}
}

func TestApplyProviderMatchReplacesArtworkAndRemovesOldCache(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	images.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(testJPEG)),
			Request:    req,
		}, nil
	})}
	scraper.SetImageProxy(images)

	libPath := t.TempDir()
	lib := model.Library{Name: "Movies", Path: libPath, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	oldPoster := "https://image.tmdb.org/t/p/w500/old-cache-poster.jpg"
	newPoster := "https://image.tmdb.org/t/p/w500/new-cache-poster.jpg"
	if err := images.PrefetchRemote(t.Context(), oldPoster); err != nil {
		t.Fatal(err)
	}
	_, oldCachePath, _, err := images.remoteImageCachePaths(oldPoster)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldCachePath); err != nil {
		t.Fatalf("old poster cache should exist before replace: %v", err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Raw",
		Path:         filepath.Join(libPath, "raw.mkv"),
		PosterURL:    oldPoster,
		ScrapeStatus: "matched",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	match := &Match{Title: "Matched", PosterURL: newPoster}
	if err := scraper.applyProviderMatch(t.Context(), &media, &lib, match); err != nil {
		t.Fatal(err)
	}

	var stored model.Media
	if err := repos.DB.First(&stored, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PosterURL != newPoster {
		t.Fatalf("poster = %q, want %q", stored.PosterURL, newPoster)
	}
	if _, err := os.Stat(oldCachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old poster cache should be removed, stat err=%v", err)
	}
	_, newCachePath, _, err := images.remoteImageCachePaths(newPoster)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(newCachePath); err != nil {
		t.Fatalf("new poster cache should exist: %v", err)
	}
}
