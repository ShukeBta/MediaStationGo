package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
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
	want := filepath.Join(dest, "电影", "Dune (2021)", "Dune (2021).mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized file at %q: %v", want, err)
	}
}

func TestOrganizeDirectoryDryRunReturnsPreviewWithoutWriting(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Dune 2021 2160p WEB-DL.mkv")
	writeOrgFile(t, sourceFile, "dune-uhd")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("organize directory dry-run: %v", err)
	}
	if !res.DryRun || res.Organized != 1 || len(res.Items) != 1 {
		t.Fatalf("unexpected dry-run result: %+v", res)
	}
	want := filepath.Join(dest, "电影", "Dune (2021)", "Dune (2021).mkv")
	if res.Items[0].Source != sourceFile || res.Items[0].Target != want || res.Items[0].Action != "organize" {
		t.Fatalf("preview item = %#v, want source=%q target=%q action=organize", res.Items[0], sourceFile, want)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create %q, stat err=%v", want, err)
	}
}

func TestOrganizeDirectoryHonorsManualMediaType(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "Some Show S01E01 2024 1080p.mkv"), "show-e01")
	writeOrgFile(t, filepath.Join(src, "Some Movie 2024 1080p.mkv"), "movie")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	tvRes, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   filepath.Join(src, "Some Show S01E01 2024 1080p.mkv"),
		DestPath:     dest,
		MediaType:    "tv",
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize manual tv type: %v", err)
	}
	if tvRes.Organized != 1 {
		t.Fatalf("tv result = %+v, want organized=1", tvRes)
	}
	tvWant := filepath.Join(dest, "电视剧", "Some Show", "Season 01", "Some Show - S01E01.mkv")
	if _, err := os.Stat(tvWant); err != nil {
		t.Fatalf("expected tv file at %q: %v", tvWant, err)
	}

	movieRes, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   filepath.Join(src, "Some Movie 2024 1080p.mkv"),
		DestPath:     dest,
		MediaType:    "movie",
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize manual movie type: %v", err)
	}
	if movieRes.Organized != 1 {
		t.Fatalf("movie result = %+v, want organized=1", movieRes)
	}
	movieWant := filepath.Join(dest, "电影", "Some Movie (2024)", "Some Movie (2024).mkv")
	if _, err := os.Stat(movieWant); err != nil {
		t.Fatalf("expected movie file at %q: %v", movieWant, err)
	}
}

func TestOrganizeDirectoryRedirectsManualStagingDest(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	// 目标指向"手动整理"暂存目录:媒体应归入父级媒体根的分类目录,
	// 而不是停留在 手动整理/ 下,也不是作为分类目录的兄弟目录。
	dest := filepath.Join(root, "media", "手动整理")
	writeOrgFile(t, filepath.Join(src, "Some Movie 2024 1080p.mkv"), "movie")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   filepath.Join(src, "Some Movie 2024 1080p.mkv"),
		DestPath:     dest,
		MediaType:    "movie",
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize into staging dest: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("result = %+v, want organized=1", res)
	}
	want := filepath.Join(root, "media", "电影", "Some Movie (2024)", "Some Movie (2024).mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized movie at %q: %v; items=%+v", want, err, res.Items)
	}
	if _, err := os.Stat(filepath.Join(dest, "电影")); err == nil {
		t.Fatalf("media must not be nested under the 手动整理 staging dir")
	}
}

func TestOrganizeDirectoryHonorsAdultMediaTypeRoot(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "ABP-123.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "adult")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		MediaType:    "adult",
		TransferMode: TransferCopy,
		DryRun:       true,
	})
	if err != nil {
		t.Fatalf("organize adult type: %v", err)
	}
	if res.Organized != 1 || len(res.Items) != 1 {
		t.Fatalf("result = %+v, want one preview item", res)
	}
	if !pathWithin(res.Items[0].Target, filepath.Join(dest, "成人")) {
		t.Fatalf("adult target = %q, want under %q", res.Items[0].Target, filepath.Join(dest, "成人"))
	}
	if res.Items[0].MediaType != "adult" {
		t.Fatalf("media type = %q, want adult", res.Items[0].MediaType)
	}
}
