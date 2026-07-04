package service

import (
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSchedulerCloudSyncImportsMountedCloudLibrary(t *testing.T) {
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		if path != "/" {
			t.Fatalf("unexpected openlist path %q", path)
		}
		return []openListTestEntry{{Name: "Cloud.Movie.2026.mkv", Size: 1024}}, 1
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	local := model.Library{Name: "电影", Path: "/media/电影", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &local); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList · 电影", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "cloud.auto_sync_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	scheduler := NewSchedulerService(log, repos, scanner, nil, nil, storage, NewHub(log), "")
	scheduler.now = func() time.Time { return fixedNightlySyncTime() }

	if err := scheduler.jobSyncCloudLibraries(t.Context()); err != nil {
		t.Fatalf("cloud sync: %v", err)
	}
	var media model.Media
	if err := repos.DB.First(&media, "path = ?", "cloud://openlist/Cloud.Movie.2026.mkv").Error; err != nil {
		t.Fatalf("cloud media not imported: %v", err)
	}
	if media.STRMURL != "/api/cloud/play/openlist?ref=%2FCloud.Movie.2026.mkv" {
		t.Fatalf("strm url = %q", media.STRMURL)
	}
}

func TestSchedulerCloudSyncRunsOnlyOnceInsideNightlyWindow(t *testing.T) {
	var requests atomic.Int32
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		requests.Add(1)
		if path != "/" {
			t.Fatalf("unexpected openlist path %q", path)
		}
		return []openListTestEntry{{Name: "Nightly.Cloud.Movie.2026.mkv", Size: 1024}}, 1
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), cloudAutoSyncEnabledKey, "true"); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	scheduler := NewSchedulerService(log, repos, scanner, nil, nil, storage, NewHub(log), "")

	scheduler.now = func() time.Time {
		return time.Date(2026, 6, 11, 22, 30, 0, 0, time.Local)
	}
	if err := scheduler.jobSyncCloudLibraries(t.Context()); err != nil {
		t.Fatalf("cloud sync outside window: %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("outside nightly window made %d requests, want 0", got)
	}

	scheduler.now = func() time.Time { return fixedNightlySyncTime() }
	if err := scheduler.jobSyncCloudLibraries(t.Context()); err != nil {
		t.Fatalf("cloud sync inside window: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("inside nightly window requests = %d, want 1", got)
	}
	if got := countMedia(t, repos); got != 1 {
		t.Fatalf("media count = %d, want 1", got)
	}

	scheduler.now = func() time.Time {
		return time.Date(2026, 6, 12, 4, 15, 0, 0, time.Local)
	}
	if !cloudAutoSyncInWindow(scheduler.now()) {
		t.Fatalf("04:15 should still be inside overnight cloud sync window")
	}
	if got := cloudAutoSyncWindowDate(scheduler.now()); got != "2026-06-11" {
		t.Fatalf("04:15 should belong to previous nightly window, got %s", got)
	}
	if err := scheduler.jobSyncCloudLibraries(t.Context()); err != nil {
		t.Fatalf("second cloud sync same overnight window: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("same overnight auto sync should not rerun, requests = %d", got)
	}

	scheduler.now = func() time.Time {
		return time.Date(2026, 6, 12, 5, 0, 0, 0, time.Local)
	}
	if cloudAutoSyncInWindow(scheduler.now()) {
		t.Fatalf("05:00 should be outside overnight cloud sync window")
	}
}

func TestSchedulerRunNowCloudSyncBypassesNightlyWindow(t *testing.T) {
	var requests atomic.Int32
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		requests.Add(1)
		return []openListTestEntry{{Name: "Manual.Cloud.Movie.2026.mkv", Size: 1024}}, 1
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	scheduler := NewSchedulerService(log, repos, scanner, nil, nil, storage, NewHub(log), "")
	scheduler.now = func() time.Time {
		return time.Date(2026, 6, 11, 10, 0, 0, 0, time.Local)
	}
	scheduler.jobs = []*scheduledJob{{
		name:     "cloud_sync",
		interval: time.Minute,
		run:      scheduler.jobSyncCloudLibraries,
	}}

	if err := scheduler.RunNow(t.Context(), "cloud_sync"); err != nil {
		t.Fatalf("manual cloud sync: %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("manual cloud sync requests = %d, want 1", got)
	}
}

func TestSchedulerCloudSyncDisabledByDefault(t *testing.T) {
	var requests atomic.Int32
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		requests.Add(1)
		return []openListTestEntry{{Name: "Cloud.Movie.2026.mkv", Size: 1024}}, 1
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	scheduler := NewSchedulerService(log, repos, scanner, nil, nil, storage, NewHub(log), "")

	if err := scheduler.jobSyncCloudLibraries(t.Context()); err != nil {
		t.Fatalf("disabled cloud sync should be a no-op: %v", err)
	}
	if got := requests.Load(); got != 0 {
		t.Fatalf("cloud sync made %d upstream requests while disabled by default", got)
	}
	if got := countMedia(t, repos); got != 0 {
		t.Fatalf("media count = %d, want 0 while cloud sync disabled by default", got)
	}
}

func fixedNightlySyncTime() time.Time {
	return time.Date(2026, 6, 11, 23, 30, 0, 0, time.Local)
}
