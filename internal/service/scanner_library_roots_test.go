package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestScanLibraryScansMultipleRootsAndPrunesPerRoot(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	fileA := filepath.Join(rootA, "电影A.2024.mkv")
	fileB := filepath.Join(rootB, "电影B.2025.mkv")
	writeTestFile(t, fileA, "a")
	writeTestFile(t, fileB, "b")

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	lib := &model.Library{Name: "电影", Path: rootA, Type: "movie", Enabled: true}
	roots := []model.LibraryRoot{
		{Name: "硬盘1", Path: rootA, Enabled: true, SortOrder: 0},
		{Name: "硬盘2", Path: rootB, Enabled: true, SortOrder: 1},
	}
	if err := repos.Library.CreateWithRoots(t.Context(), lib, roots); err != nil {
		t.Fatal(err)
	}

	scanner := NewScannerService(nil, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan multiple roots: %v", err)
	}
	if res.Added != 2 {
		t.Fatalf("added = %d, want 2", res.Added)
	}
	var rows []model.Media
	if err := db.Order("path asc").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("media rows = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.LibraryRootID == "" {
			t.Fatalf("media %q missing library_root_id", row.Path)
		}
		if row.RelativePath == "" {
			t.Fatalf("media %q missing relative_path", row.Path)
		}
	}

	removeTestPath(t, fileA)
	removeTestPath(t, rootB)
	res, err = scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan with one offline root should continue: %v", err)
	}
	if res.Removed != 1 {
		t.Fatalf("removed = %d, want only vanished file from accessible root removed", res.Removed)
	}
	rows = nil
	if err := db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Path != fileB {
		t.Fatalf("remaining rows = %#v, want offline root media preserved", rows)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func removeTestPath(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}
