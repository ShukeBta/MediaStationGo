package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

func TestWatchDirectoryTraversalIgnoresFilesAndSupportsCancellation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a/nested", ".hidden/child"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	file := filepath.Join(root, "z.mp4")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	var dirs []string
	err := walkDirsForWatch(t.Context(), root, func(path string) {
		dirs = append(dirs, path)
		if path == filepath.Join(root, "a") {
			if err := os.Remove(file); err != nil {
				t.Fatal(err)
			}
		}
	})
	if err != nil || !reflect.DeepEqual(dirs, []string{root, filepath.Join(root, "a"), filepath.Join(root, "a/nested")}) {
		t.Fatalf("dirs=%v err=%v", dirs, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := walkDirsForWatch(ctx, root, func(string) { t.Error("visited after cancellation") }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := walkDirsForWatch(t.Context(), filepath.Join(root, "missing"), func(string) {}); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

func TestWatchDirectoryTraversalFollowsLinksWithoutCycles(t *testing.T) {
	root, other := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(other, "child"), 0755); err != nil {
		t.Fatal(err)
	}
	createTestSymlink(t, other, filepath.Join(root, "linked"))
	createTestSymlink(t, root, filepath.Join(other, "cycle"))
	dirs := listDirsForWatch(root)
	if !reflect.DeepEqual(dirs, []string{root, filepath.Join(root, "linked"), filepath.Join(root, "linked", "child")}) {
		t.Fatalf("link dirs: %v", dirs)
	}
}

func TestWatcherMissingDiskDoesNotDeleteMedia(t *testing.T) {
	scanner, repos := newScannerTestEnv(t)
	root := filepath.Join(t.TempDir(), "disk")
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "disk", Path: root, Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "movie.mkv")
	media := model.Media{LibraryID: lib.ID, Title: "saved", Path: file}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, root+"-offline"); err != nil {
		t.Fatal(err)
	}
	w := NewWatcherService(zap.NewNop(), repos, scanner)
	w.process(t.Context(), duePath{path: file, libraryID: lib.ID})
	if countMedia(t, repos) != 1 {
		t.Fatal("offline disk deleted library entry")
	}
	if len(w.pending) != 1 {
		t.Fatal("offline path was not queued for retry")
	}
	if err := os.Mkdir(root, 0755); err != nil {
		t.Fatal(err)
	}
	w.process(t.Context(), duePath{path: file, libraryID: lib.ID})
	if countMedia(t, repos) != 0 {
		t.Fatal("confirmed deletion was not applied")
	}
}

func TestWatcherMissingLinkedDiskDoesNotDeleteMedia(t *testing.T) {
	scanner, repos := newScannerTestEnv(t)
	root, base := t.TempDir(), t.TempDir()
	disk := filepath.Join(base, "disk")
	if err := os.Mkdir(disk, 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "linked")
	createTestSymlink(t, disk, link)
	lib := model.Library{Name: "linked disk", Path: root, Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(link, "movie.mkv")
	if err := repos.DB.Create(&model.Media{LibraryID: lib.ID, Title: "saved", Path: file}).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(disk, disk+"-offline"); err != nil {
		t.Fatal(err)
	}
	w := NewWatcherService(zap.NewNop(), repos, scanner)
	w.process(t.Context(), duePath{path: file, libraryID: lib.ID})
	if countMedia(t, repos) != 1 {
		t.Fatal("broken linked disk deleted catalog entry")
	}
}
