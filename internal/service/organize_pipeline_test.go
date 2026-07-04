package service

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeScanRootUsesActualOrganizedTarget(t *testing.T) {
	target := filepath.Join(string(filepath.Separator), "media", "电影", "动画电影", "Big Buck Bunny (2008)", "Big Buck Bunny (2008).mp4")
	res := &OrganizeResult{
		DestPath: filepath.Join(string(filepath.Separator), "media"),
		Items: []OrganizePreviewItem{{
			Target: target,
			Action: "organize",
		}},
	}

	want := filepath.Dir(target)
	if got := organizeScanRoot(res, ""); got != want {
		t.Fatalf("organizeScanRoot() = %q, want %q", got, want)
	}
}

func TestOrganizeScanRootUsesCommonAffectedCategoryRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "media", "电影", "动画电影")
	res := &OrganizeResult{
		DestPath: filepath.Join(string(filepath.Separator), "media"),
		Items: []OrganizePreviewItem{
			{Target: filepath.Join(root, "Movie A (2026)", "Movie A (2026).mp4"), Action: "organize"},
			{Target: filepath.Join(root, "Movie B (2026)", "Movie B (2026).mp4"), Action: "organize"},
		},
	}

	if got := organizeScanRoot(res, ""); got != root {
		t.Fatalf("organizeScanRoot() = %q, want %q", got, root)
	}
}

func TestOrganizePipelineFailsWhenEveryOrganizeItemErrors(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "Dune 2021 1080p.mkv"), "movie")

	origLinkFile := linkFile
	linkFile = func(_, _ string) error {
		return errors.New("cross-device link")
	}
	t.Cleanup(func() { linkFile = origLinkFile })

	repos := newOrganizerTestRepo(t)
	organizer := NewOrganizerService(nil, zap.NewNop(), repos)
	tasks := NewTaskTrackerService(zap.NewNop(), nil)
	pipeline := NewOrganizePipelineService(zap.NewNop(), repos, organizer, nil, tasks)

	_, err := pipeline.Run(t.Context(), OrganizePipelineRequest{
		Scope:        OrganizeScopeDirectory,
		Trigger:      OrganizeTriggerScheduled,
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: string(TransferHardlink),
	})
	if err == nil {
		t.Fatal("expected organize pipeline to fail when every item errors")
	}
	if !strings.Contains(err.Error(), "organize failed") || !strings.Contains(err.Error(), "hardlink failed") {
		t.Fatalf("error = %q, want organize failure with hardlink detail", err.Error())
	}
	snap := tasks.Snapshot()
	if len(snap.Recent) != 1 {
		t.Fatalf("recent tasks = %d, want 1", len(snap.Recent))
	}
	task := snap.Recent[0]
	if task.Status != TaskStatusFailed {
		t.Fatalf("task status = %q, want failed", task.Status)
	}
	if !strings.Contains(task.Error, "hardlink failed") {
		t.Fatalf("task error = %q, want hardlink detail", task.Error)
	}
	if len(task.Details) == 0 || !strings.Contains(task.Details[0], "cross-device link") {
		t.Fatalf("task details = %#v, want transfer error detail", task.Details)
	}
}

func TestOrganizePipelinePrefersCurrentMediaLibraryAfterMediaOrganize(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	sourceLib := model.Library{Name: "待整理", Path: filepath.Join(t.TempDir(), "incoming"), Type: "tv", Enabled: true}
	targetLib := model.Library{Name: "国产剧", Path: filepath.Join(t.TempDir(), "media", "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &sourceLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &targetLib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    targetLib.ID,
		Title:        "南部档案",
		Path:         filepath.Join(targetLib.Path, "南部档案", "Season 01", "南部档案 - S01E01.mkv"),
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	pipeline := NewOrganizePipelineService(zap.NewNop(), repos, nil, nil, nil)
	got := pipeline.scanPreferredLibraryID(t.Context(), OrganizePipelineRequest{
		Scope:   OrganizeScopeMedia,
		MediaID: media.ID,
	})
	if got != targetLib.ID {
		t.Fatalf("preferred library = %q, want current media library %q", got, targetLib.ID)
	}

	got = pipeline.scanPreferredLibraryID(t.Context(), OrganizePipelineRequest{
		Scope:              OrganizeScopeMedia,
		MediaID:            media.ID,
		PreferredLibraryID: sourceLib.ID,
	})
	if got != sourceLib.ID {
		t.Fatalf("explicit preferred library = %q, want %q", got, sourceLib.ID)
	}
}
