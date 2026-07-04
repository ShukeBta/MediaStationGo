package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

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
	existing := filepath.Join(dest, "电影", "The Matrix (1999)", "The Matrix (1999).mkv")
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

func TestOrganizeDirectoryTreatsScannedTargetPathAsLibraryDuplicate(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	writeOrgFile(t, filepath.Join(src, "Weird.Release.2024.1080p.mkv"), "source")
	existing := filepath.Join(dest, "电影", "Weird Release (2024)", "Weird Release (2024).mkv")
	writeOrgFile(t, existing, "existing")

	repos := newOrganizerTestRepo(t)
	row := model.Media{Title: "刮削后的正式片名", Path: existing, Year: 2024, Container: "mkv", Width: 1920, Height: 1080}
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
	if res.Organized != 0 || res.Skipped != 1 {
		t.Fatalf("result = %+v, want organized=0 skipped=1", res)
	}
	if len(res.Items) != 1 || res.Items[0].Reason != organizeSkipDuplicateLibrary {
		t.Fatalf("items = %+v, want duplicate in library", res.Items)
	}
	if OrganizeResultNeedsVisibilitySync(res) {
		t.Fatal("path already present in DB should not trigger another visibility scan")
	}
}

func TestOrganizeDirectorySkipsSampleClips(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	writeOrgFile(t, filepath.Join(src, "Demo.Movie.2024.1080p.mkv"), "main")
	writeOrgFile(t, filepath.Join(src, "Samples", "Sample1.mkv"), "sample-dir")
	writeOrgFile(t, filepath.Join(src, "Demo.Movie.2024.sample.mkv"), "sample-file")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 1 || res.Skipped != 2 {
		t.Fatalf("result = %+v, want organized=1 skipped=2", res)
	}
	if got := OrganizeSkipReasonCounts(res)[organizeSkipSampleClip]; got != 2 {
		t.Fatalf("sample skip count = %d, want 2; items=%+v", got, res.Items)
	}
	if _, err := os.Stat(filepath.Join(dest, "电影", "Sample1", "Sample1.mkv")); !os.IsNotExist(err) {
		t.Fatalf("sample directory clip should not be organized, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "电影", "Demo Movie Sample (2024)", "Demo Movie Sample (2024).mkv")); !os.IsNotExist(err) {
		t.Fatalf("sample suffix clip should not be organized, stat err=%v", err)
	}
}

// TestOrganizeDirectorySkipsHigherResolutionWhenReplacementDisabled verifies
// that dedup wins by default: even a higher-resolution source must not replace
// an existing library item unless the caller explicitly allows washing.
func TestOrganizeDirectorySkipsHigherResolutionWhenReplacementDisabled(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	// Source is 2160p; filename token drives resolutionArea when no DB row.
	writeOrgFile(t, filepath.Join(src, "Inception 2010 2160p BluRay.mkv"), "inception-uhd")
	// Destination already holds an organized 1080p version (scanned dims).
	existing := filepath.Join(dest, "电影", "Inception (2010)", "Inception (2010).mkv")
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
	if res.Skipped != 1 || res.Replaced != 0 || res.Organized != 0 {
		t.Fatalf("expected skipped=1 replaced=0 organized=0 (wash disabled), got %+v", res)
	}
	got, err := os.ReadFile(existing)
	if err != nil || string(got) != "inception-1080p" {
		t.Fatalf("destination must keep existing version when wash disabled, got %q err=%v", string(got), err)
	}
	var count int64
	if err := repos.DB.Model(&model.Media{}).Where("path = ?", existing).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected existing media DB row kept, found %d", count)
	}
}

// TestOrganizeDirectoryReplaceHigherResolutionWhenAllowed verifies 洗版: a
// higher-resolution source replaces the lower-resolution version already in the
// destination only when the caller explicitly allows replacement.
func TestOrganizeDirectoryReplaceHigherResolutionWhenAllowed(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	writeOrgFile(t, filepath.Join(src, "Inception 2010 2160p BluRay.mkv"), "inception-uhd")
	existing := filepath.Join(dest, "电影", "Inception (2010)", "Inception (2010).mkv")
	writeOrgFile(t, existing, "inception-1080p")

	repos := newOrganizerTestRepo(t)
	row := model.Media{Title: "Inception", Path: existing, Year: 2010, Container: "mkv", Width: 1920, Height: 1080}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:           src,
		DestPath:             dest,
		TransferMode:         TransferCopy,
		AllowReplaceExisting: true,
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
	existing := filepath.Join(dest, "电影", "Inception (2010)", "Inception (2010).mkv")
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

func TestOrganizeDirectoryDoesNotReclassifySameEpisodeVersionInTargetDir(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	source := filepath.Join(src, "南部档案.S01E16.1080p.IQ.WEB-DL.H264.AAC-UBWEB.mkv")
	writeOrgFile(t, source, "download-1080p")

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organize.tv_format", "{title}/Season {season:02}/{title} - {episode_tag}{% if video_format %}-{{video_format}}{% endif %}{fileExt}"); err != nil {
		t.Fatal(err)
	}

	lib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}

	existing := filepath.Join(lib.Path, "南部档案", "Season 01", "南部档案 - S01E16-2160p.IQ.WEB-DL.H265.DDP5.1-UBWEB.mkv")
	writeOrgFile(t, existing, "library-2160p")
	row := model.Media{
		LibraryID:    lib.ID,
		Title:        "南部档案",
		Path:         existing,
		Container:    "mkv",
		SeasonNum:    1,
		EpisodeNum:   16,
		Width:        3840,
		Height:       2160,
		TMDbID:       123456,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:           src,
		DestPath:             dest,
		MediaType:            "tv",
		MediaCategory:        "国产剧",
		TransferMode:         TransferCopy,
		AllowReplaceExisting: false,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Reclassified != 0 || res.Replaced != 0 || res.Organized != 0 || res.Skipped != 1 {
		t.Fatalf("result = %+v, want skipped duplicate without reclassify", res)
	}
	if _, err := os.Stat(existing); err != nil {
		t.Fatalf("existing higher version should stay at original path: %v", err)
	}
	wrongRename := filepath.Join(lib.Path, "南部档案", "Season 01", "南部档案 - S01E16-1080p.IQ.WEB-DL.H264.AAC-UBWEB.mkv")
	if _, err := os.Stat(wrongRename); !os.IsNotExist(err) {
		t.Fatalf("existing version must not be renamed to incoming release path, stat err=%v", err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", existing).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "南部档案" || got.TMDbID != 123456 || got.ScrapeStatus != "matched" {
		t.Fatalf("metadata changed after duplicate organize: title=%q tmdb=%d status=%q", got.Title, got.TMDbID, got.ScrapeStatus)
	}
}

// TestOrganizeDirectoryTVEpisodeDedup verifies per-episode dedup for TV media.
func TestOrganizeDirectoryTVEpisodeDedup(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")

	writeOrgFile(t, filepath.Join(src, "Friends S01E01 1080p.mkv"), "friends-s01e01-src")
	existing := filepath.Join(dest, "电视剧", "Friends", "Season 01", "Friends - S01E01.mkv")
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
	e02 := filepath.Join(dest, "电视剧", "Friends", "Season 01", "Friends - S01E02.mkv")
	if _, err := os.Stat(e02); err != nil {
		t.Fatalf("expected E02 organized at %q: %v", e02, err)
	}
}
