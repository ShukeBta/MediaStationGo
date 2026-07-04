package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestSchedulerOrganizeSourceDisabledByDefault(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "国产剧", "狂飙.S01E01.2023.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organize.source_dir", src); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organize.target_dir", dest); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organize.transfer_mode", "copy"); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	scheduler := NewSchedulerService(zap.NewNop(), repos, nil, nil, organizer, nil, NewHub(zap.NewNop()), "")
	if err := scheduler.jobOrganizeSource(t.Context()); err != nil {
		t.Fatalf("disabled organize source job should be a no-op: %v", err)
	}

	want := filepath.Join(dest, "电视剧", "国产剧", "狂飙", "Season 01", "狂飙 - S01E01.mkv")
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("disabled job should not create %q, stat err=%v", want, err)
	}
}

func TestSchedulerOrganizeSourceUsesConfiguredSourceAndDestination(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "国产剧", "狂飙.S01E01.2023.1080p.mkv"), "episode")

	repos := newOrganizerTestRepo(t)
	for key, value := range map[string]string{
		"organize.auto":          "true",
		"organize.source_dir":    src,
		"organize.target_dir":    dest,
		"organize.transfer_mode": "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	scheduler := NewSchedulerService(zap.NewNop(), repos, nil, nil, organizer, nil, NewHub(zap.NewNop()), "")
	if err := scheduler.jobOrganizeSource(t.Context()); err != nil {
		t.Fatalf("organize source job: %v", err)
	}

	want := filepath.Join(dest, "电视剧", "国产剧", "狂飙", "Season 01", "狂飙 - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected scheduled organize output at %q: %v", want, err)
	}
}

func TestSchedulerRunNowOrganizeSourceBypassesDisabledSwitch(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "Dune 2021 1080p.mkv"), "movie")

	repos := newOrganizerTestRepo(t)
	for key, value := range map[string]string{
		"organize.source_dir":    src,
		"organize.target_dir":    dest,
		"organize.transfer_mode": "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	scheduler := NewSchedulerService(zap.NewNop(), repos, nil, nil, organizer, nil, NewHub(zap.NewNop()), "")
	scheduler.jobs = []*scheduledJob{{
		name:     "organize_source",
		interval: time.Minute,
		run:      scheduler.jobOrganizeSource,
	}}

	if err := scheduler.RunNow(t.Context(), "organize_source"); err != nil {
		t.Fatalf("run now organize source: %v", err)
	}

	want := filepath.Join(dest, "电影", "Dune (2021)", "Dune (2021).mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected run-now organize output at %q: %v", want, err)
	}
}

func TestSchedulerOrganizeSourceSyncsVisibilityWhenTargetAlreadyExists(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "国产剧", "狂飙.S01E01.2023.1080p.mkv"), "episode")

	repos := newOrganizerTestRepo(t)
	for key, value := range map[string]string{
		"organize.source_dir":    src,
		"organize.target_dir":    dest,
		"organize.transfer_mode": "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	lib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	if _, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	}); err != nil {
		t.Fatalf("seed organize destination: %v", err)
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	scheduler := NewSchedulerService(zap.NewNop(), repos, scanner, nil, organizer, nil, NewHub(zap.NewNop()), "")
	scheduler.jobs = []*scheduledJob{{
		name:     "organize_source",
		interval: time.Minute,
		run:      scheduler.jobOrganizeSource,
	}}

	if err := scheduler.RunNow(t.Context(), "organize_source"); err != nil {
		t.Fatalf("run now organize source: %v", err)
	}
	var count int64
	if err := repos.DB.Model(&model.Media{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("target already exists should still be scanned into DB, count=%d want 1", count)
	}
}
