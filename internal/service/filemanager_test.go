package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func newFileManagerTestServiceWithRepo(t *testing.T, root string) (*FileManagerService, *repository.Container) {
	t.Helper()
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "downloads", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.App.DataDir = root
	cfg.Cache.CacheDir = root
	return NewFileManagerService(cfg, zap.NewNop(), repos), repos
}

func newFileManagerTestService(t *testing.T, root string) *FileManagerService {
	t.Helper()
	svc, _ := newFileManagerTestServiceWithRepo(t, root)
	return svc
}

func TestFileManagerRecursiveListAndMutations(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "downloads", "国产剧")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(nested, "狂飙.S01E01.mkv")
	if err := os.WriteFile(mediaPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newFileManagerTestService(t, root)

	listing, err := svc.List(root, 100, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range listing.Entries {
		if entry.Path == mediaPath {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("recursive listing did not include %s", mediaPath)
	}

	created, err := svc.CreateFolder(root, "整理目标")
	if err != nil {
		t.Fatal(err)
	}
	emptyListing, err := svc.List(created.Path, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if emptyListing.Entries == nil || len(emptyListing.Entries) != 0 {
		t.Fatalf("empty directory entries = %#v, want empty slice", emptyListing.Entries)
	}
	copied, err := svc.Transfer(mediaPath, created.Path, TransferCopy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(copied.Path); err != nil {
		t.Fatalf("copied file missing: %v", err)
	}
	renamed, err := svc.Rename(copied.Path, "狂飙 - S01E01.mkv")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamed.Path); err != nil {
		t.Fatalf("renamed file missing: %v", err)
	}
	if err := svc.Delete(renamed.Path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(renamed.Path); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}
}

func TestFileManagerTransferDirectoryHardlinksFiles(t *testing.T) {
	root := t.TempDir()
	if hardlinksUnsupported(t, root) {
		t.Skip("hardlinks unsupported on this filesystem")
	}
	sourceDir := filepath.Join(root, "downloads", "Show")
	seasonDir := filepath.Join(sourceDir, "Season 01")
	targetRoot := filepath.Join(root, "media")
	if err := os.MkdirAll(seasonDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	sourceFile := filepath.Join(seasonDir, "Show.S01E01.mkv")
	if err := os.WriteFile(sourceFile, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newFileManagerTestService(t, root)

	res, err := svc.Transfer(sourceDir, targetRoot, TransferHardlink)
	if err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(res.Path, "Season 01", "Show.S01E01.mkv")
	sourceInfo, err := os.Stat(sourceFile)
	if err != nil {
		t.Fatal(err)
	}
	targetInfo, err := os.Stat(targetFile)
	if err != nil {
		t.Fatalf("hardlinked directory file missing: %v", err)
	}
	if !os.SameFile(sourceInfo, targetInfo) {
		t.Fatal("directory hardlink should hardlink contained files")
	}
	listing, err := svc.List(res.Path, 100, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range listing.Entries {
		if entry.Path == targetFile {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("target directory listing did not include %s", targetFile)
	}
}

func TestFileManagerRefusesRootMutation(t *testing.T) {
	root := t.TempDir()
	svc := newFileManagerTestService(t, root)
	if err := svc.Delete(root); !errors.Is(err, ErrRootMutation) {
		t.Fatalf("Delete(root) err = %v, want ErrRootMutation", err)
	}
}

func hardlinksUnsupported(t *testing.T, root string) bool {
	t.Helper()
	src := filepath.Join(root, "hardlink-probe-src")
	dst := filepath.Join(root, "hardlink-probe-dst")
	if err := os.WriteFile(src, []byte("probe"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := os.Link(src, dst)
	_ = os.Remove(src)
	_ = os.Remove(dst)
	return err != nil
}

func TestFileManagerIncludesConfiguredOrganizeRoots(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "downloads")
	targetDir := filepath.Join(root, "media")
	qbDir := filepath.Join(root, "qb-save")
	for _, dir := range []string{sourceDir, targetDir, qbDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	svc, repos := newFileManagerTestServiceWithRepo(t, root)
	if err := repos.Setting.Set(t.Context(), "organize.source_dir", sourceDir); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "organize.target_dir", targetDir); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), "qbittorrent.savepath", qbDir); err != nil {
		t.Fatal(err)
	}

	listing, err := svc.List("", 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, root := range listing.Roots {
		got[root.Label] = root.Path
	}
	for label, want := range map[string]string{
		"organize-source": filepath.Clean(sourceDir),
		"organize-target": filepath.Clean(targetDir),
		"qb-savepath":     filepath.Clean(qbDir),
	} {
		if got[label] != want {
			t.Fatalf("root %s = %q, want %q; roots=%#v", label, got[label], want, listing.Roots)
		}
	}
}

func TestFileManagerIncludesAllLibraryRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	nestedB := filepath.Join(rootB, "second-root")
	if err := os.MkdirAll(nestedB, 0o755); err != nil {
		t.Fatal(err)
	}
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	lib := &model.Library{Name: "电影", Path: rootA, Type: "movie", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), lib, []model.LibraryRoot{
		{Name: "硬盘1", Path: rootA, Enabled: true},
		{Name: "硬盘2", Path: rootB, Enabled: true, SortOrder: 1},
	}); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	cfg.Cache.CacheDir = t.TempDir()
	svc := NewFileManagerService(cfg, zap.NewNop(), repos)

	listing, err := svc.List("", 100)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, root := range listing.Roots {
		got[root.Label] = root.Path
	}
	if got["library:电影:硬盘1"] != filepath.Clean(rootA) {
		t.Fatalf("root A missing from listing: %#v", listing.Roots)
	}
	if got["library:电影:硬盘2"] != filepath.Clean(rootB) {
		t.Fatalf("root B missing from listing: %#v", listing.Roots)
	}
	if _, err := svc.List(nestedB, 100); err != nil {
		t.Fatalf("list second library root: %v", err)
	}
}
