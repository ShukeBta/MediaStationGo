package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func writeOrgFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestOrganizeDirectoryNewMedia organizes a brand-new movie from a source
// directory (e.g. the download dir) into the destination — no library row
// required.
func TestOrganizeDirectoryNewMedia(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "Dune 2021 2160p WEB-DL.mkv"), "dune-uhd")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 1 || res.Replaced != 0 || res.Skipped != 0 {
		t.Fatalf("expected organized=1 replaced=0 skipped=0, got %+v", res)
	}
	want := filepath.Join(dest, "Dune (2021)", "Dune (2021).mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized file at %q: %v", want, err)
	}
}

// TestOrganizeDirectoryDedup verifies that media already present in the
// destination is NOT organized again from the source (去重), and the existing
// file is left untouched.
func TestOrganizeDirectoryDedup(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	// Source release at 1080p.
	writeOrgFile(t, filepath.Join(src, "The Matrix 1999 1080p BluRay.mkv"), "matrix-source")
	// Destination already holds the organized 1080p version.
	existing := filepath.Join(dest, "The Matrix (1999)", "The Matrix (1999).mkv")
	writeOrgFile(t, existing, "matrix-existing")

	repos := newOrganizerTestRepo(t)
	// Scanned destination row carries real 1080p dimensions.
	row := model.Media{Title: "The Matrix", Path: existing, Year: 1999, Container: "mkv", Width: 1920, Height: 1080}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 0 || res.Replaced != 0 || res.Skipped != 1 {
		t.Fatalf("expected organized=0 replaced=0 skipped=1 (dedup), got %+v", res)
	}
	// Existing destination file must be untouched.
	got, err := os.ReadFile(existing)
	if err != nil || string(got) != "matrix-existing" {
		t.Fatalf("existing file must be untouched, got %q err=%v", string(got), err)
	}
}

// TestOrganizeDirectoryReplaceHigherResolution verifies 洗版: a higher-resolution
// source replaces the lower-resolution version already in the destination.
func TestOrganizeDirectoryReplaceHigherResolution(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	// Source is 2160p; filename token drives resolutionArea when no DB row.
	writeOrgFile(t, filepath.Join(src, "Inception 2010 2160p BluRay.mkv"), "inception-uhd")
	// Destination already holds an organized 1080p version (scanned dims).
	existing := filepath.Join(dest, "Inception (2010)", "Inception (2010).mkv")
	writeOrgFile(t, existing, "inception-1080p")

	repos := newOrganizerTestRepo(t)
	row := model.Media{Title: "Inception", Path: existing, Year: 2010, Container: "mkv", Width: 1920, Height: 1080}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Replaced != 1 || res.Organized != 0 || res.Skipped != 0 {
		t.Fatalf("expected replaced=1 organized=0 skipped=0 (洗版), got %+v", res)
	}
	// Destination file must now contain the higher-resolution source content.
	got, err := os.ReadFile(existing)
	if err != nil || string(got) != "inception-uhd" {
		t.Fatalf("destination must hold the higher-res source content, got %q err=%v", string(got), err)
	}
	// The replaced DB row should be gone.
	var count int64
	if err := repos.DB.Model(&model.Media{}).Where("path = ?", existing).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected replaced media DB row removed, found %d", count)
	}
}

// TestOrganizeDirectoryKeepsHigherResolutionExisting verifies that a LOWER-res
// source does NOT replace a higher-res existing file (it is treated as a
// duplicate and skipped).
func TestOrganizeDirectoryKeepsHigherResolutionExisting(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	writeOrgFile(t, filepath.Join(src, "Inception 2010 720p.mkv"), "inception-720p")
	existing := filepath.Join(dest, "Inception (2010)", "Inception (2010).mkv")
	writeOrgFile(t, existing, "inception-2160p")

	repos := newOrganizerTestRepo(t)
	row := model.Media{Title: "Inception", Path: existing, Year: 2010, Container: "mkv", Width: 3840, Height: 2160}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Skipped != 1 || res.Replaced != 0 || res.Organized != 0 {
		t.Fatalf("expected skipped=1 (keep higher-res existing), got %+v", res)
	}
	got, err := os.ReadFile(existing)
	if err != nil || string(got) != "inception-2160p" {
		t.Fatalf("higher-res existing must be kept, got %q err=%v", string(got), err)
	}
}

// TestOrganizeDirectoryTVEpisodeDedup verifies per-episode dedup for TV media.
func TestOrganizeDirectoryTVEpisodeDedup(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	writeOrgFile(t, filepath.Join(src, "Friends S01E01 1080p.mkv"), "friends-s01e01-src")
	existing := filepath.Join(dest, "Friends", "Season 01", "Friends - S01E01.mkv")
	writeOrgFile(t, existing, "friends-s01e01-existing")
	// A different episode that should still be organized fresh.
	writeOrgFile(t, filepath.Join(src, "Friends S01E02 1080p.mkv"), "friends-s01e02-src")

	repos := newOrganizerTestRepo(t)
	row := model.Media{Title: "Friends", Path: existing, SeasonNum: 1, EpisodeNum: 1, Container: "mkv", Width: 1920, Height: 1080}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	// E01 deduped (skipped), E02 organized fresh.
	if res.Organized != 1 || res.Skipped != 1 || res.Replaced != 0 {
		t.Fatalf("expected organized=1 skipped=1 replaced=0, got %+v", res)
	}
	if got, err := os.ReadFile(existing); err != nil || string(got) != "friends-s01e01-existing" {
		t.Fatalf("existing E01 must be untouched, got %q err=%v", string(got), err)
	}
	e02 := filepath.Join(dest, "Friends", "Season 01", "Friends - S01E02.mkv")
	if _, err := os.Stat(e02); err != nil {
		t.Fatalf("expected E02 organized at %q: %v", e02, err)
	}
}
