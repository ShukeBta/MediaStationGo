package service

import (
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSchedulerPeriodicLocalScanRunsAtMostOncePerDay(t *testing.T) {
	root := t.TempDir()
	libraryPath := filepath.Join(root, "library")
	writeOrgFile(t, filepath.Join(libraryPath, "Daily.Show.S01E01.mkv"), "episode 1")

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "scan.periodic_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "本地剧集", Path: libraryPath, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scheduler := NewSchedulerService(log, repos, scanner, nil, nil, nil, NewHub(log), "")
	scheduler.now = func() time.Time {
		return time.Date(2026, 6, 20, 10, 0, 0, 0, time.Local)
	}

	if err := scheduler.jobScanLibraries(t.Context()); err != nil {
		t.Fatalf("first periodic local scan: %v", err)
	}
	if got := countMedia(t, repos); got != 1 {
		t.Fatalf("media count after first scan = %d, want 1", got)
	}

	writeOrgFile(t, filepath.Join(libraryPath, "Daily.Show.S01E02.mkv"), "episode 2")
	if err := scheduler.jobScanLibraries(t.Context()); err != nil {
		t.Fatalf("same-day periodic local scan: %v", err)
	}
	if got := countMedia(t, repos); got != 1 {
		t.Fatalf("same-day periodic scan should not import new file, media count = %d", got)
	}

	scheduler.now = func() time.Time {
		return time.Date(2026, 6, 21, 10, 0, 0, 0, time.Local)
	}
	if err := scheduler.jobScanLibraries(t.Context()); err != nil {
		t.Fatalf("next-day periodic local scan: %v", err)
	}
	if got := countMedia(t, repos); got != 2 {
		t.Fatalf("media count after next-day scan = %d, want 2", got)
	}
}

func TestSchedulerManualLocalScanBypassesDailyPeriodicLimit(t *testing.T) {
	root := t.TempDir()
	libraryPath := filepath.Join(root, "library")
	writeOrgFile(t, filepath.Join(libraryPath, "Manual.Show.S01E01.mkv"), "episode 1")

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "scan.periodic_enabled", "true"); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "手动本地剧集", Path: libraryPath, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scheduler := NewSchedulerService(log, repos, scanner, nil, nil, nil, NewHub(log), "")
	scheduler.now = func() time.Time {
		return time.Date(2026, 6, 20, 10, 0, 0, 0, time.Local)
	}
	scheduler.jobs = []*scheduledJob{{
		name:     "library_scan",
		interval: 24 * time.Hour,
		run:      scheduler.jobScanLibraries,
	}}

	if err := scheduler.jobScanLibraries(t.Context()); err != nil {
		t.Fatalf("first periodic local scan: %v", err)
	}
	writeOrgFile(t, filepath.Join(libraryPath, "Manual.Show.S01E02.mkv"), "episode 2")
	if err := scheduler.RunNow(t.Context(), "library_scan"); err != nil {
		t.Fatalf("manual local scan: %v", err)
	}
	if got := countMedia(t, repos); got != 2 {
		t.Fatalf("manual scan should bypass daily periodic limit, media count = %d", got)
	}
}
