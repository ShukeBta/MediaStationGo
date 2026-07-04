package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestOrganizeDirectoryUsesConfiguredSourceWhenRequestSourceEmpty(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "Dune 2021 2160p WEB-DL.mkv"), "dune-uhd")

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organize.source_dir", src); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory with configured source: %v", err)
	}
	if res.SourcePath != filepath.Clean(src) || res.Organized != 1 {
		t.Fatalf("result = %+v, want source=%q organized=1", res, src)
	}
}

func TestOrganizeDirectorySkipsActiveQBitTorrentDownloads(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	active := filepath.Join(src, "Still.Downloading.2026.1080p.mkv")
	done := filepath.Join(src, "Dune 2021 2160p WEB-DL.mkv")
	writeOrgFile(t, active, "partial")
	writeOrgFile(t, done, "done")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	org.SetActiveDownloadProvider(func(context.Context) []QBitTorrent {
		return []QBitTorrent{{
			Hash:        "active",
			Name:        "Still.Downloading.2026.1080p",
			State:       "downloading",
			Progress:    0.42,
			SavePath:    src,
			ContentPath: active,
		}}
	})
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferMove,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 1 || res.Skipped != 1 {
		t.Fatalf("result = %+v, want organized=1 skipped=1", res)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("active qB file must stay in source: %v", err)
	}
	for _, item := range res.Items {
		if item.Source == active && item.Action == "skip" && item.Reason == organizeSkipActiveDownload {
			return
		}
	}
	t.Fatalf("missing active-download skip item: %+v", res.Items)
}

func TestOrganizeDirectoryMapsConfiguredHostPathsToContainerPaths(t *testing.T) {
	root := t.TempDir()
	hostDownloads := filepath.Join(root, "nas-host", "downloads")
	hostMedia := filepath.Join(root, "nas-host", "media")
	containerDownloads := filepath.Join(root, "container", "downloads")
	containerMedia := filepath.Join(root, "container", "media")
	containerSource := filepath.Join(containerDownloads, "国产剧")
	writeOrgFile(t, filepath.Join(containerSource, "Some Show S01E01 2024 1080p.mkv"), "show-e01")

	t.Setenv("MEDIASTATION_DOWNLOAD_DIR", hostDownloads)
	t.Setenv("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", containerDownloads)
	t.Setenv("MEDIASTATION_MEDIA_DIR", hostMedia)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerMedia)

	repos := newOrganizerTestRepo(t)
	for key, value := range map[string]string{
		"organize.source_dir":    filepath.Join(hostDownloads, "国产剧"),
		"organize.target_dir":    hostMedia,
		"organize.transfer_mode": "copy",
	} {
		if err := repos.Setting.Set(t.Context(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{})
	if err != nil {
		t.Fatalf("organize mapped host paths: %v", err)
	}
	if res.SourcePath != filepath.Clean(containerSource) || res.DestPath != filepath.Clean(containerMedia) {
		t.Fatalf("result paths = source %q dest %q, want %q -> %q", res.SourcePath, res.DestPath, containerSource, containerMedia)
	}
	want := filepath.Join(containerMedia, "电视剧", "国产剧", "Some Show", "Season 01", "Some Show - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized file at %q: %v", want, err)
	}
}

func TestOrganizeDirectoryAcceptsSingleVideoFileSource(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Dune 2021 2160p WEB-DL.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "dune-uhd")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize single file: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("result = %+v, want organized=1", res)
	}
	want := filepath.Join(dest, "电影", "Dune (2021)", "Dune (2021).mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized file at %q: %v", want, err)
	}
}

func TestOrganizeSourceCandidatesOnlyReturnAccessibleDirectories(t *testing.T) {
	root := t.TempDir()
	configuredDir := filepath.Join(root, "configured-downloads")
	downloadDir := filepath.Join(root, "downloads")
	mediaDir := filepath.Join(root, "media")
	if err := os.MkdirAll(configuredDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", downloadDir)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", filepath.Join(root, "missing-media"))

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organize.source_dir", configuredDir); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	candidates := org.OrganizeSourceCandidates(t.Context())
	if len(candidates) != 2 {
		t.Fatalf("candidates = %#v, want configured source + accessible download dir", candidates)
	}
	if candidates[0].Path != filepath.Clean(configuredDir) || candidates[0].Kind != "source" {
		t.Fatalf("first candidate = %#v, want configured source %q", candidates[0], configuredDir)
	}
	if candidates[1].Path != filepath.Clean(downloadDir) || candidates[1].Kind != "download" {
		t.Fatalf("second candidate = %#v, want accessible download dir %q", candidates[1], downloadDir)
	}

	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", mediaDir)
	candidates = org.OrganizeSourceCandidates(t.Context())
	if len(candidates) != 3 {
		t.Fatalf("candidates = %#v, want configured source, download and media dirs", candidates)
	}
}
