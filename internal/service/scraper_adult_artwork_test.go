package service

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestDeriveAdultPosterIfNeededFromBackdrop(t *testing.T) {
	backdrop := image.NewRGBA(image.Rect(0, 0, 1200, 600))
	for y := 0; y < 600; y++ {
		for x := 0; x < 1200; x++ {
			backdrop.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 80, A: 255})
		}
	}
	cacheDir := t.TempDir()
	backdropPath := filepath.Join(cacheDir, "backdrop.jpg")
	file, err := os.Create(backdropPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, backdrop, &jpeg.Options{Quality: 90}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: cacheDir}}, zap.NewNop())
	scraper := &ScraperService{images: images, log: zap.NewNop()}
	match := &Match{MediaType: "adult", BackdropURL: backdropPath}

	scraper.deriveAdultPosterIfNeeded(t.Context(), &model.Media{Base: model.Base{ID: "media-1"}}, &model.Library{Type: "adult"}, match)

	if match.PosterURL == "" {
		t.Fatal("poster was not generated")
	}
	if !strings.Contains(match.PosterURL, "adult-posters") {
		t.Fatalf("poster path should use adult-posters cache: %s", match.PosterURL)
	}
	data, _, err := images.Fetch(t.Context(), match.PosterURL)
	if err != nil {
		t.Fatal(err)
	}
	assertJPEGJFIFHeader(t, data)
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 600 || img.Bounds().Dy() != 900 {
		t.Fatalf("poster size = %dx%d, want 600x900", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestDeriveAdultPosterIfPosterMatchesBackdrop(t *testing.T) {
	cacheDir := t.TempDir()
	backdropPath := filepath.Join(cacheDir, "same-cover.jpg")
	writeAdultArtworkTestImage(t, backdropPath, 800, 538)

	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: cacheDir}}, zap.NewNop())
	scraper := &ScraperService{images: images, log: zap.NewNop()}
	match := &Match{MediaType: "adult", PosterURL: backdropPath, BackdropURL: backdropPath}

	scraper.deriveAdultPosterIfNeeded(t.Context(), &model.Media{Base: model.Base{ID: "media-2"}}, &model.Library{Type: "adult"}, match)

	if match.PosterURL == backdropPath {
		t.Fatal("poster should be regenerated when poster and backdrop are the same landscape image")
	}
	if !strings.Contains(match.BackdropURL, "adult-backdrops") {
		t.Fatalf("backdrop should be normalized to adult-backdrops cache: %s", match.BackdropURL)
	}
	assertAdultPosterSize(t, images, match.PosterURL)
	assertAdultBackdropSize(t, images, match.BackdropURL, 800, 538)
}

func TestDeriveAdultPosterFromLandscapePosterWithoutBackdrop(t *testing.T) {
	cacheDir := t.TempDir()
	posterPath := filepath.Join(cacheDir, "landscape-cover.jpg")
	writeAdultArtworkTestImage(t, posterPath, 800, 538)

	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: cacheDir}}, zap.NewNop())
	scraper := &ScraperService{images: images, log: zap.NewNop()}
	match := &Match{MediaType: "adult", PosterURL: posterPath}

	scraper.deriveAdultPosterIfNeeded(t.Context(), &model.Media{Base: model.Base{ID: "media-poster-only"}}, &model.Library{Type: "adult"}, match)

	if !strings.Contains(match.PosterURL, "adult-posters") {
		t.Fatalf("poster should be regenerated from landscape poster: %s", match.PosterURL)
	}
	if !strings.Contains(match.BackdropURL, "adult-backdrops") {
		t.Fatalf("landscape poster should also produce a backdrop: %s", match.BackdropURL)
	}
	assertAdultPosterSize(t, images, match.PosterURL)
	assertAdultBackdropSize(t, images, match.BackdropURL, 800, 538)
}

func TestDeriveAdultPosterKeepsPortraitPosterWithoutBackdrop(t *testing.T) {
	cacheDir := t.TempDir()
	posterPath := filepath.Join(cacheDir, "portrait-poster.jpg")
	writeAdultArtworkTestImage(t, posterPath, 600, 900)

	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: cacheDir}}, zap.NewNop())
	scraper := &ScraperService{images: images, log: zap.NewNop()}
	match := &Match{MediaType: "adult", PosterURL: posterPath}

	scraper.deriveAdultPosterIfNeeded(t.Context(), &model.Media{Base: model.Base{ID: "media-portrait-only"}}, &model.Library{Type: "adult"}, match)

	if match.PosterURL != posterPath {
		t.Fatalf("portrait poster should stay unchanged: %s", match.PosterURL)
	}
	if match.BackdropURL != "" {
		t.Fatalf("portrait poster should not produce a backdrop: %s", match.BackdropURL)
	}
}

func TestDeriveAdultPosterKeepsPortraitPoster(t *testing.T) {
	cacheDir := t.TempDir()
	posterPath := filepath.Join(cacheDir, "poster.jpg")
	backdropPath := filepath.Join(cacheDir, "backdrop.jpg")
	writeAdultArtworkTestImage(t, posterPath, 600, 900)
	writeAdultArtworkTestImage(t, backdropPath, 800, 538)

	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: cacheDir}}, zap.NewNop())
	scraper := &ScraperService{images: images, log: zap.NewNop()}
	match := &Match{MediaType: "adult", PosterURL: posterPath, BackdropURL: backdropPath}

	scraper.deriveAdultPosterIfNeeded(t.Context(), &model.Media{Base: model.Base{ID: "media-3"}}, &model.Library{Type: "adult"}, match)

	if match.PosterURL != posterPath {
		t.Fatalf("portrait poster should stay unchanged: %s", match.PosterURL)
	}
}

func TestDeriveAdultPosterFallsBackToPosterForBadBackdrop(t *testing.T) {
	cacheDir := t.TempDir()
	landscapePath := filepath.Join(cacheDir, "landscape-cover.jpg")
	badBackdropPath := filepath.Join(cacheDir, "tiny-backdrop.jpg")
	writeAdultArtworkTestImage(t, landscapePath, 800, 538)
	writeAdultArtworkTestImage(t, badBackdropPath, 90, 122)

	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: cacheDir}}, zap.NewNop())
	scraper := &ScraperService{images: images, log: zap.NewNop()}
	match := &Match{MediaType: "adult", PosterURL: landscapePath, BackdropURL: badBackdropPath}

	scraper.deriveAdultPosterIfNeeded(t.Context(), &model.Media{Base: model.Base{ID: "media-4"}}, &model.Library{Type: "adult"}, match)

	if !strings.Contains(match.PosterURL, "adult-posters") {
		t.Fatalf("poster should be regenerated from landscape source: %s", match.PosterURL)
	}
	if !strings.Contains(match.BackdropURL, "adult-backdrops") {
		t.Fatalf("bad backdrop should be replaced with local landscape backdrop: %s", match.BackdropURL)
	}
	assertAdultPosterSize(t, images, match.PosterURL)
	out, _, err := images.Fetch(t.Context(), match.BackdropURL)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 800 || img.Bounds().Dy() != 538 {
		t.Fatalf("backdrop size = %dx%d, want 800x538", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func TestWriteAdultBackdropNormalizesJPEG(t *testing.T) {
	cacheDir := t.TempDir()
	backdropPath := filepath.Join(cacheDir, "dmm-backdrop.jpg")
	writeAdultArtworkTestImage(t, backdropPath, 800, 534)
	data, err := os.ReadFile(backdropPath)
	if err != nil {
		t.Fatal(err)
	}
	images := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: cacheDir}}, zap.NewNop())
	scraper := &ScraperService{images: images, log: zap.NewNop()}

	path, err := scraper.writeAdultBackdrop("https://pics.dmm.co.jp/digital/video/ssis00721/ssis00721jp-1.jpg", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, "adult-backdrops") {
		t.Fatalf("backdrop path should use adult-backdrops cache: %s", path)
	}
	out, _, err := images.Fetch(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	assertJPEGJFIFHeader(t, out)
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 800 || img.Bounds().Dy() != 534 {
		t.Fatalf("backdrop size = %dx%d, want 800x534", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func writeAdultArtworkTestImage(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 80, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(file, img, &jpeg.Options{Quality: 90}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertJPEGJFIFHeader(t *testing.T, data []byte) {
	t.Helper()
	if len(data) < 11 {
		t.Fatalf("poster data too short: %d bytes", len(data))
	}
	if data[0] != 0xff || data[1] != 0xd8 || data[2] != 0xff || data[3] != 0xe0 || string(data[6:11]) != "JFIF\x00" {
		t.Fatalf("poster should start with JFIF JPEG header, got % x", data[:min(len(data), 16)])
	}
}

func assertAdultPosterSize(t *testing.T, images *ImageProxy, path string) {
	t.Helper()
	data, _, err := images.Fetch(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 600 || img.Bounds().Dy() != 900 {
		t.Fatalf("poster size = %dx%d, want 600x900", img.Bounds().Dx(), img.Bounds().Dy())
	}
}

func assertAdultBackdropSize(t *testing.T, images *ImageProxy, path string, width, height int) {
	t.Helper()
	data, _, err := images.Fetch(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != width || img.Bounds().Dy() != height {
		t.Fatalf("backdrop size = %dx%d, want %dx%d", img.Bounds().Dx(), img.Bounds().Dy(), width, height)
	}
}
