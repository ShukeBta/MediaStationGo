package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func newScannerTestEnv(t *testing.T) (*ScannerService, *repository.Container) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	sc := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	return sc, repos
}

func countMedia(t *testing.T, repos *repository.Container) int64 {
	t.Helper()
	var n int64
	if err := repos.DB.Model(&model.Media{}).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}

func TestIngestPathAddsSingleFile(t *testing.T) {
	sc, repos := newScannerTestEnv(t)
	root := t.TempDir()
	lib := model.Library{Name: "Movies", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "Some Movie (2021).mkv")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := sc.IngestPath(t.Context(), lib.ID, file)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	if !added {
		t.Fatal("expected file to be added")
	}
	if got := countMedia(t, repos); got != 1 {
		t.Fatalf("media count = %d, want 1", got)
	}
	// Non-video file is ignored.
	other := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(other, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if added, _ := sc.IngestPath(t.Context(), lib.ID, other); added {
		t.Fatal("non-video file should not be ingested")
	}
}

func TestRemovePathDeletesVanishedMedia(t *testing.T) {
	sc, repos := newScannerTestEnv(t)
	root := t.TempDir()
	lib := model.Library{Name: "Movies", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "Gone (2020).mkv")
	if err := os.WriteFile(file, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := sc.IngestPath(t.Context(), lib.ID, file); err != nil {
		t.Fatal(err)
	}
	if countMedia(t, repos) != 1 {
		t.Fatal("expected 1 media before removal")
	}
	// A still-present file is not removed.
	if removed, _ := sc.RemovePath(t.Context(), file); removed != 0 {
		t.Fatalf("present file should not be removed, got %d", removed)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	removed, err := sc.RemovePath(t.Context(), file)
	if err != nil {
		t.Fatalf("remove: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if countMedia(t, repos) != 0 {
		t.Fatal("expected 0 media after removal")
	}
}

// TestScanSkipsHardlinkDuplicate verifies that a hardlink (same inode) kept
// for seeding is not imported as a second media item, preventing duplicate
// recognition and double-counted storage.
func TestScanSkipsHardlinkDuplicate(t *testing.T) {
	sc, repos := newScannerTestEnv(t)
	root := t.TempDir()
	lib := model.Library{Name: "Movies", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	orig := filepath.Join(root, "Movie (2019).mkv")
	if err := os.WriteFile(orig, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	linked := filepath.Join(root, "Movie (2019) [organized].mkv")
	if err := os.Link(orig, linked); err != nil {
		t.Skipf("hardlinks unsupported on this fs: %v", err)
	}
	res, err := sc.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if got := countMedia(t, repos); got != 1 {
		t.Fatalf("media count = %d, want 1 (hardlink should be deduped)", got)
	}
	if res.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", res.Skipped)
	}
}
