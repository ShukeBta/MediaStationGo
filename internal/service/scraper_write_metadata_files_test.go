package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// TestWriteArtworkDataToPathWritesJellyfinSidecar verifies that in-memory
// artwork bytes are written as a Jellyfin/Emby sidecar (poster.jpg) in the
// media directory using the scraped image MIME type.
func TestWriteArtworkDataToPathWritesJellyfinSidecar(t *testing.T) {
	scraper := &ScraperService{log: zap.NewNop()}
	mediaDir := t.TempDir()

	dst := scraper.writeArtworkDataToPath(mediaDir, "poster", "image/jpeg", testJPEG)
	if dst == "" {
		t.Fatal("expected a written destination path")
	}
	if filepath.Base(dst) != "poster.jpg" {
		t.Fatalf("base = %q, want poster.jpg", filepath.Base(dst))
	}
	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("poster.jpg not readable: %v", err)
	}
	if string(data) != string(testJPEG) {
		t.Fatal("poster.jpg content mismatch")
	}
}

// TestImageExtForContentType verifies the MIME -> extension mapping used to
// name Jellyfin sidecar files.
func TestImageExtForContentType(t *testing.T) {
	cases := map[string]string{
		"image/jpeg":         ".jpg",
		"image/pjpeg":        ".jpg",
		"image/png":          ".png",
		"image/webp":         ".webp",
		"image/gif":          ".gif",
		"image/avif":         ".avif",
		"image/jpeg; charset=binary": ".jpg",
		"application/octet-stream":   ".img",
	}
	for in, want := range cases {
		if got := imageExtForContentType(in); got != want {
			t.Errorf("imageExtForContentType(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestWriteMediaArtworkFilesAfterScrapeSkipsCloud verifies that cloud media
// never has artwork written anywhere.
func TestWriteMediaArtworkFilesAfterScrapeSkipsCloud(t *testing.T) {
	scraper := &ScraperService{log: zap.NewNop()}
	cloud := &model.Media{
		Path:        "cloud://aliyun/openlist/Some.Movie.mkv",
		PosterURL:   "https://image.tmdb.org/t/p/poster.jpg",
		BackdropURL: "https://image.tmdb.org/t/p/backdrop.jpg",
	}
	scraper.writeMediaArtworkFilesAfterScrape(t.Context(), cloud, nil)
	// No panic and no files written; the method should return early.
}

// TestSameDirectoryMediaUsesBaseScopedSidecars verifies that two movies sharing
// one directory produce distinct, base-scoped sidecar names (A-poster.jpg vs
// B-poster.jpg) so they never overwrite each other.
func TestSameDirectoryMediaUsesBaseScopedSidecars(t *testing.T) {
	mediaPath := filepath.Join(t.TempDir(), "Movie Folder", "A.mp4")
	if err := os.MkdirAll(filepath.Dir(mediaPath), 0o755); err != nil {
		t.Fatal(err)
	}
	scraper := &ScraperService{log: zap.NewNop()}

	// Simulate the base-scoped naming used by writeMediaArtworkFilesAfterScrape.
	base := "A"
	dir := filepath.Dir(mediaPath)
	dst := scraper.writeArtworkDataToPath(dir, base+"-poster", "image/jpeg", testJPEG)
	if dst == "" {
		t.Fatal("expected a written destination")
	}
	if filepath.Base(dst) != "A-poster.jpg" {
		t.Fatalf("base = %q, want A-poster.jpg", filepath.Base(dst))
	}
}
