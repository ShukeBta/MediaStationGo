package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func createTestSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func TestScanLibraryFollowsDirectorySymlinksWithoutCycles(t *testing.T) {
	scanner, repos := newScannerTestEnv(t)
	root, disk := t.TempDir(), t.TempDir()
	writeTestFile(t, filepath.Join(root, "Local Movie.mkv"), "local")
	writeTestFile(t, filepath.Join(disk, "Movies", "Linked Movie.iso"), "linked ISO bytes")
	link := filepath.Join(root, "Disk 2")
	createTestSymlink(t, disk, link)
	createTestSymlink(t, root, filepath.Join(disk, "cycle"))
	lib := model.Library{Name: "Linked disks", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil || res.Added != 2 {
		t.Fatalf("scan = %#v, %v; want both local and linked media", res, err)
	}
	var media model.Media
	linkedPath := filepath.Join(link, "Movies", "Linked Movie.iso")
	if err := repos.DB.Where("path = ?", linkedPath).First(&media).Error; err != nil {
		t.Fatal(err)
	}
	if media.SizeBytes != int64(len("linked ISO bytes")) {
		t.Fatalf("size = %d, want target file size", media.SizeBytes)
	}
	dirs := listDirsForWatch(root)
	found := false
	for _, dir := range dirs {
		found = found || dir == filepath.Join(link, "Movies")
	}
	if !found {
		t.Fatalf("linked media directories not watched: %v", dirs)
	}
	res, err = scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil || res.Added != 0 || res.Removed != 0 || countMedia(t, repos) != 2 {
		t.Fatalf("rescan duplicated or removed linked media: %#v, %v", res, err)
	}
}

func TestScanLibraryAcceptsSymlinkRootAndFile(t *testing.T) {
	scanner, repos := newScannerTestEnv(t)
	root, disk := t.TempDir(), t.TempDir()
	file := filepath.Join(disk, "Source.mkv")
	writeTestFile(t, file, "media file content")
	linkRoot := filepath.Join(root, "Library")
	createTestSymlink(t, disk, linkRoot)
	lib := model.Library{Name: "Root link", Path: linkRoot, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil || res.Added != 1 {
		t.Fatalf("symlink root scan = %#v, %v", res, err)
	}
	fileRoot := t.TempDir()
	fileLink := filepath.Join(fileRoot, "Linked Movie.mkv")
	createTestSymlink(t, file, fileLink)
	fileLib := model.Library{Name: "File link", Path: fileRoot, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &fileLib); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ScanLibrary(t.Context(), fileLib.ID); err != nil {
		t.Fatal(err)
	}
	var media model.Media
	if err := repos.DB.Where("path = ?", fileLink).First(&media).Error; err != nil {
		t.Fatal(err)
	}
	if media.SizeBytes != int64(len("media file content")) {
		t.Fatalf("linked file size = %d, want target size", media.SizeBytes)
	}
}

func TestScanLibraryPreservesMediaWhenLinkedDiskGoesOffline(t *testing.T) {
	scanner, repos := newScannerTestEnv(t)
	root, diskParent := t.TempDir(), t.TempDir()
	disk := filepath.Join(diskParent, "disk")
	writeTestFile(t, filepath.Join(disk, "Offline Movie.mkv"), "media")
	createTestSymlink(t, disk, filepath.Join(root, "Disk 2"))
	lib := model.Library{Name: "Offline disk", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ScanLibrary(t.Context(), lib.ID); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(disk, filepath.Join(diskParent, "offline")); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(root, "Still Online.mkv"), "online")
	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err == nil || res.ErrorCount == 0 {
		t.Fatalf("offline link must be reported: %#v, %v", res, err)
	}
	if res.Added != 1 || res.Removed != 0 || countMedia(t, repos) != 2 {
		t.Fatalf("partial scan must import online media and preserve offline media: %#v", res)
	}
}

func TestScanLibraryImportsHardlinkWhenPreviousPathDisappears(t *testing.T) {
	scanner, repos := newScannerTestEnv(t)
	root := t.TempDir()
	primary := filepath.Join(root, "A Movie.mkv")
	linked := filepath.Join(root, "B Movie.mkv")
	writeTestFile(t, primary, "movie bytes")
	if err := os.Link(primary, linked); err != nil {
		t.Skipf("hardlinks unavailable: %v", err)
	}
	lib := model.Library{Name: "Hardlinks", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ScanLibrary(t.Context(), lib.ID); err != nil {
		t.Fatal(err)
	}
	var original model.Media
	if err := repos.DB.Where("library_id = ?", lib.ID).First(&original).Error; err != nil {
		t.Fatal(err)
	}
	// Keep the inode linked twice, but move the old primary out of the library.
	if err := os.Rename(original.Path, filepath.Join(t.TempDir(), "Moved.mkv")); err != nil {
		t.Fatal(err)
	}
	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil || res.Added != 1 || res.Removed != 1 || countMedia(t, repos) != 1 {
		t.Fatalf("remaining hardlink must be imported: %#v, %v", res, err)
	}
}

func TestScanLibraryAcceptsExplicitHiddenRoot(t *testing.T) {
	scanner, repos := newScannerTestEnv(t)
	root := filepath.Join(t.TempDir(), ".library")
	writeTestFile(t, filepath.Join(root, "Visible.mkv"), "movie")
	writeTestFile(t, filepath.Join(root, ".cache", "Ignored.mkv"), "cache")
	lib := model.Library{Name: "Hidden root", Path: root, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil || res.Added != 1 {
		t.Fatalf("explicit hidden root scan = %#v, %v", res, err)
	}
}
