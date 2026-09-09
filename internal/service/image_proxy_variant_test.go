package service

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestImageProxyServeRespectsEmbyImageVariantQuery(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "poster.jpg")
	writeTestJPEG(t, path, 200, 300)

	proxy := NewImageProxy(&config.Config{
		App:   config.AppConfig{DataDir: root},
		Cache: config.CacheConfig{CacheDir: t.TempDir()},
	}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/Items/media-1/Images/Primary?maxWidth=50&quality=70", nil)
	rec := httptest.NewRecorder()

	if err := proxy.Serve(context.Background(), rec, req, path); err != nil {
		t.Fatalf("serve image: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", rec.Code, rec.Body.String())
	}
	if contentType := rec.Header().Get("Content-Type"); !strings.Contains(contentType, "image/jpeg") {
		t.Fatalf("Content-Type = %q, want jpeg", contentType)
	}
	img, err := jpeg.Decode(rec.Body)
	if err != nil {
		t.Fatalf("decode response jpeg: %v", err)
	}
	if got := img.Bounds().Dx(); got != 50 {
		t.Fatalf("width = %d, want 50", got)
	}
	if got := img.Bounds().Dy(); got != 75 {
		t.Fatalf("height = %d, want 75", got)
	}
	if got := rec.Header().Get("Cache-Control"); got != imageBrowserCacheControl {
		t.Fatalf("Cache-Control = %q, want %q", got, imageBrowserCacheControl)
	}
}

func TestImageVariantResponseDoesNotWaitForCacheMaintenance(t *testing.T) {
	for _, maintenance := range []string{"variant", "original"} {
		t.Run(maintenance, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "poster.jpg")
			writeTestJPEG(t, path, 200, 300)
			p := NewImageProxy(&config.Config{
				App:   config.AppConfig{DataDir: root},
				Cache: config.CacheConfig{CacheDir: t.TempDir()},
			}, zap.NewNop())
			req := httptest.NewRequest(http.MethodGet, "/Images/Primary?maxWidth=50&quality=70", nil)
			rec := httptest.NewRecorder()
			// Maintenance owns this mutex for the whole directory walk. The response
			// must complete while it is still held, not merely shortly after release.
			maintenanceMu := &p.variantCacheMu
			if maintenance == "original" {
				maintenanceMu = &p.mu
			}
			maintenanceMu.Lock()
			done := make(chan error, 1)
			go func() {
				p.scheduleImageVariantCachePrune(false)
				done <- p.Serve(req.Context(), rec, req, path)
			}()
			select {
			case err := <-done:
				maintenanceMu.Unlock()
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(2 * time.Second):
				maintenanceMu.Unlock()
				<-done
				t.Fatal("image response blocked on cache maintenance")
			}
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d", rec.Code)
			}
			img, err := jpeg.Decode(bytes.NewReader(rec.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if img.Bounds().Dx() != 50 || img.Bounds().Dy() != 75 {
				t.Fatal("wrong image dimensions")
			}
			if p.variantCacheBytes != 0 {
				t.Fatal("busy cache admitted a write")
			}
			second := httptest.NewRecorder()
			if err := p.Serve(req.Context(), second, req, path); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(rec.Body.Bytes(), second.Body.Bytes()) {
				t.Fatal("cache contention changed response")
			}
			if p.variantCacheBytes == 0 {
				t.Fatal("cache did not accept subsequent write")
			}
		})
	}
}

func TestImageProxyServeUsesFallbackForMalformedJPEG(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "broken.jpg")
	writeTestJPEG(t, path, 200, 300)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jpeg: %v", err)
	}
	broken := append(append([]byte(nil), original[:len(original)/2]...), make([]byte, 4096)...)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatalf("write broken jpeg: %v", err)
	}

	proxy := NewImageProxy(&config.Config{
		App:   config.AppConfig{DataDir: root},
		Cache: config.CacheConfig{CacheDir: t.TempDir()},
	}, zap.NewNop())
	fallbackCalled := false
	proxy.variantFallback = func(_ context.Context, _ []byte, variant imageVariantOptions) ([]byte, string, error) {
		fallbackCalled = true
		if variant.maxWidth != 50 || variant.quality != 70 {
			t.Fatalf("variant = %+v", variant)
		}
		return testJPEGBytes(t, 50, 75), "image/jpeg", nil
	}

	req := httptest.NewRequest(http.MethodGet, "/Items/media-1/Images/Primary?maxWidth=50&quality=70", nil)
	rec := httptest.NewRecorder()
	if err := proxy.Serve(context.Background(), rec, req, path); err != nil {
		t.Fatalf("serve image: %v", err)
	}
	if !fallbackCalled {
		t.Fatal("malformed jpeg did not use variant fallback")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q", rec.Code, rec.Body.String())
	}
	if bytes.Equal(rec.Body.Bytes(), broken) {
		t.Fatal("variant response silently returned malformed original")
	}
	img, err := jpeg.Decode(rec.Body)
	if err != nil {
		t.Fatalf("decode fallback jpeg: %v", err)
	}
	if got := img.Bounds().Dx(); got != 50 {
		t.Fatalf("width = %d, want 50", got)
	}
}

func TestImageProxyServeRejectsMalformedJPEGWhenFallbackFails(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "broken.jpg")
	broken := []byte{0xff, 0xd8, 0xff, 0xdb, 0x00, 0x04, 0x00, 0x00}
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatalf("write broken jpeg: %v", err)
	}

	proxy := NewImageProxy(&config.Config{
		App:   config.AppConfig{DataDir: root},
		Cache: config.CacheConfig{CacheDir: t.TempDir()},
	}, zap.NewNop())
	proxy.variantFallback = func(context.Context, []byte, imageVariantOptions) ([]byte, string, error) {
		return nil, "", errors.New("fallback failed")
	}

	req := httptest.NewRequest(http.MethodGet, "/Items/media-1/Images/Primary?maxWidth=50&quality=70", nil)
	rec := httptest.NewRecorder()
	if err := proxy.Serve(context.Background(), rec, req, path); err != nil {
		t.Fatalf("serve image: %v", err)
	}
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	if bytes.Equal(rec.Body.Bytes(), broken) {
		t.Fatal("failed variant response returned malformed original")
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
}

func TestImageProxyCachesImageVariantsBySourceVersionAndOptions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "broken.jpg")
	writeTestJPEG(t, path, 200, 300)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read jpeg: %v", err)
	}
	broken := append(append([]byte(nil), original[:len(original)/2]...), make([]byte, 4096)...)
	if err := os.WriteFile(path, broken, 0o600); err != nil {
		t.Fatalf("write broken jpeg: %v", err)
	}

	proxy := NewImageProxy(&config.Config{
		App:   config.AppConfig{DataDir: root},
		Cache: config.CacheConfig{CacheDir: t.TempDir()},
	}, zap.NewNop())
	var fallbackCalls atomic.Int32
	proxy.variantFallback = func(_ context.Context, _ []byte, variant imageVariantOptions) ([]byte, string, error) {
		fallbackCalls.Add(1)
		width := variant.maxWidth
		if width == 0 {
			width = 50
		}
		return testJPEGBytes(t, width, width*3/2), "image/jpeg", nil
	}

	serve := func(method, target, ifNoneMatch string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, target, nil)
		if ifNoneMatch != "" {
			req.Header.Set("If-None-Match", ifNoneMatch)
		}
		rec := httptest.NewRecorder()
		if err := proxy.Serve(context.Background(), rec, req, path); err != nil {
			t.Fatalf("serve image: %v", err)
		}
		return rec
	}

	const primaryTarget = "/Items/media-1/Images/Primary?maxWidth=50&quality=70"
	first := serve(http.MethodGet, primaryTarget, "")
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d, body=%q", first.Code, first.Body.String())
	}
	firstETag := first.Header().Get("ETag")
	if firstETag == "" {
		t.Fatal("first response has no ETag")
	}
	if got := fallbackCalls.Load(); got != 1 {
		t.Fatalf("fallback calls after first GET = %d, want 1", got)
	}

	second := serve(http.MethodGet, primaryTarget, "")
	if second.Code != http.StatusOK {
		t.Fatalf("second status = %d, body=%q", second.Code, second.Body.String())
	}
	if got := fallbackCalls.Load(); got != 1 {
		t.Fatalf("fallback calls after cached GET = %d, want 1", got)
	}
	if got := second.Header().Get("ETag"); got != firstETag {
		t.Fatalf("cached ETag = %q, want %q", got, firstETag)
	}

	conditional := serve(http.MethodGet, primaryTarget, firstETag)
	if conditional.Code != http.StatusNotModified {
		t.Fatalf("conditional status = %d, want %d", conditional.Code, http.StatusNotModified)
	}
	if conditional.Body.Len() != 0 {
		t.Fatalf("conditional body length = %d, want 0", conditional.Body.Len())
	}
	if got := fallbackCalls.Load(); got != 1 {
		t.Fatalf("fallback calls after conditional GET = %d, want 1", got)
	}

	head := serve(http.MethodHead, primaryTarget, "")
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", head.Code, http.StatusOK)
	}
	if head.Body.Len() != 0 {
		t.Fatalf("HEAD body length = %d, want 0", head.Body.Len())
	}
	if got := fallbackCalls.Load(); got != 1 {
		t.Fatalf("fallback calls after HEAD = %d, want 1", got)
	}

	defaultQuality := serve(http.MethodGet, "/Items/media-1/Images/Primary?maxWidth=50", "")
	if defaultQuality.Code != http.StatusOK {
		t.Fatalf("default-quality status = %d, body=%q", defaultQuality.Code, defaultQuality.Body.String())
	}
	if got := fallbackCalls.Load(); got != 2 {
		t.Fatalf("fallback calls after implicit quality = %d, want 2", got)
	}

	explicitDefaultQuality := serve(http.MethodGet, "/Items/media-1/Images/Primary?maxWidth=50&quality=82", "")
	if explicitDefaultQuality.Code != http.StatusOK {
		t.Fatalf("explicit-default-quality status = %d, body=%q", explicitDefaultQuality.Code, explicitDefaultQuality.Body.String())
	}
	if got := fallbackCalls.Load(); got != 3 {
		t.Fatalf("fallback calls after explicit default quality = %d, want 3", got)
	}

	differentSize := serve(http.MethodGet, "/Items/media-1/Images/Primary?maxWidth=60&quality=70", "")
	if differentSize.Code != http.StatusOK {
		t.Fatalf("different-size status = %d, body=%q", differentSize.Code, differentSize.Body.String())
	}
	if got := fallbackCalls.Load(); got != 4 {
		t.Fatalf("fallback calls after different width = %d, want 4", got)
	}

	updated := append(append([]byte(nil), broken...), 0)
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatalf("update broken jpeg: %v", err)
	}
	updatedSource := serve(http.MethodGet, primaryTarget, "")
	if updatedSource.Code != http.StatusOK {
		t.Fatalf("updated-source status = %d, body=%q", updatedSource.Code, updatedSource.Body.String())
	}
	if got := fallbackCalls.Load(); got != 5 {
		t.Fatalf("fallback calls after source update = %d, want 5", got)
	}

	entries := imageVariantCacheFiles(t, proxy.imageVariantCacheDir())
	if len(entries) != 5 {
		t.Fatalf("variant cache entries = %d, want 5: %v", len(entries), entries)
	}
}

func TestPruneImageVariantCacheDirRemovesExpiredAndOldestEntries(t *testing.T) {
	root := t.TempDir()
	now := time.Now().Truncate(time.Second)
	write := func(name string, size int, modTime time.Time) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte(strings.Repeat("x", size)), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("set mtime for %s: %v", name, err)
		}
		return path
	}

	expired := write("expired", 3, now.Add(-31*24*time.Hour))
	oldest := write("oldest", 4, now.Add(-2*time.Hour))
	newest := write("newest", 4, now.Add(-time.Hour))
	total, err := pruneImageVariantCacheDir(root, now.Add(-30*24*time.Hour), 4)
	if err != nil {
		t.Fatalf("prune variant cache: %v", err)
	}
	if total != 4 {
		t.Fatalf("cache size after prune = %d, want 4", total)
	}
	for _, path := range []string{expired, oldest} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected %s to be removed, stat err=%v", path, err)
		}
	}
	if _, err := os.Stat(newest); err != nil {
		t.Fatalf("expected newest entry to remain: %v", err)
	}
}

func TestImageProxyStartupPruneKeepsFreshNestedEntries(t *testing.T) {
	cacheRoot := t.TempDir()
	root := filepath.Join(cacheRoot, "images", "variants")
	now := time.Now().Truncate(time.Second)
	variantPath := filepath.Join(root, "source", "version", "variant")
	if err := os.MkdirAll(filepath.Dir(variantPath), 0o750); err != nil {
		t.Fatalf("create variant directory: %v", err)
	}
	const data = "variant"
	if err := os.WriteFile(variantPath, []byte(data), 0o600); err != nil {
		t.Fatalf("write variant: %v", err)
	}
	if err := os.Chtimes(variantPath, now, now); err != nil {
		t.Fatalf("set variant mtime: %v", err)
	}

	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: cacheRoot}}, zap.NewNop())
	if proxy.variantCacheBytes != int64(len(data)) {
		t.Fatalf("cache size after startup prune = %d, want %d", proxy.variantCacheBytes, len(data))
	}
	if _, err := os.Stat(variantPath); err != nil {
		t.Fatalf("expected fresh variant to remain: %v", err)
	}
}

func TestImageProxyRemoveCachedRemovesImageVariants(t *testing.T) {
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	const raw = "https://image.tmdb.org/t/p/w500/poster.jpg"
	key, _, _, err := proxy.remoteImageCachePaths(raw)
	if err != nil {
		t.Fatalf("remote cache paths: %v", err)
	}
	_, variantPath := proxy.imageVariantCachePaths(key, time.Now(), 123, imageVariantOptions{maxWidth: 185, quality: 70, hasQuality: true})
	if err := os.MkdirAll(filepath.Dir(variantPath), 0o750); err != nil {
		t.Fatalf("create variant directory: %v", err)
	}
	if err := os.WriteFile(variantPath, []byte("variant"), 0o600); err != nil {
		t.Fatalf("write variant: %v", err)
	}
	proxy.variantCacheBytes = int64(len("variant"))

	if err := proxy.RemoveCached(raw); err != nil {
		t.Fatalf("remove cached image: %v", err)
	}
	if _, err := os.Stat(variantPath); !os.IsNotExist(err) {
		t.Fatalf("variant remains after cache removal, stat err=%v", err)
	}
	if proxy.variantCacheBytes != 0 {
		t.Fatalf("variant cache bytes = %d, want 0", proxy.variantCacheBytes)
	}
}

func imageVariantCacheFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info != nil && info.Mode().IsRegular() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatalf("walk variant cache: %v", err)
	}
	return files
}

func TestImageVariantFFmpegScaleFilter(t *testing.T) {
	tests := []struct {
		variant imageVariantOptions
		want    string
	}{
		{variant: imageVariantOptions{maxWidth: 342}, want: `scale=min(iw\,342):-2`},
		{variant: imageVariantOptions{maxHeight: 428}, want: `scale=-2:min(ih\,428)`},
		{variant: imageVariantOptions{maxWidth: 342, maxHeight: 428}, want: `scale=min(iw\,342):min(ih\,428):force_original_aspect_ratio=decrease`},
	}
	for _, tt := range tests {
		if got := imageVariantFFmpegScaleFilter(tt.variant); got != tt.want {
			t.Fatalf("filter = %q, want %q", got, tt.want)
		}
	}
}

func writeTestJPEG(t *testing.T, path string, width, height int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jpeg: %v", err)
	}
	defer f.Close()
	if err := jpeg.Encode(f, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
}

func testJPEGBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	return buf.Bytes()
}
