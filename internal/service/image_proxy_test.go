package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestImageProxyServesLocalImagePath(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "episode-thumb.png")
	if err := os.WriteFile(imagePath, transparent1x1PNG, 0o644); err != nil {
		t.Fatal(err)
	}

	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(dir, "cache")}}, zap.NewNop())
	req := httptest.NewRequest(http.MethodGet, "/api/img", nil)
	rec := httptest.NewRecorder()

	if err := proxy.Serve(t.Context(), rec, req, imagePath); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got == "" {
		t.Fatal("missing content-type")
	}
	if rec.Body.Len() != len(transparent1x1PNG) {
		t.Fatalf("body length = %d, want %d", rec.Body.Len(), len(transparent1x1PNG))
	}
}

// TestImageProxyServesPosterUnderLibraryRoot verifies that sidecar posters
// stored under an arbitrary media library root (not the configured
// data/cache/movies dirs) are served rather than dropped to the placeholder.
// This is the regression that made web/Emby posters disappear.
func TestImageProxyServesPosterUnderLibraryRoot(t *testing.T) {
	libDir := t.TempDir()
	posterPath := filepath.Join(libDir, "Inception (2010)", "poster.png")
	if err := os.MkdirAll(filepath.Dir(posterPath), 0o755); err != nil {
		t.Fatal(err)
	}
	realPoster := []byte("THIS-IS-A-REAL-POSTER-NOT-THE-PLACEHOLDER")
	if err := os.WriteFile(posterPath, realPoster, 0o644); err != nil {
		t.Fatal(err)
	}

	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())

	// Without a library-roots provider the poster lives outside every allowed
	// root, so it must fall back to the transparent placeholder.
	rec := httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), posterPath); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != len(transparent1x1PNG) {
		t.Fatalf("expected placeholder before provider set, got %d bytes", rec.Body.Len())
	}

	// Once the library root is known, the real poster bytes are served.
	proxy.SetLibraryRootsProvider(func() []string { return []string{libDir} })
	rec = httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), posterPath); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Body.Bytes(); string(got) != string(realPoster) {
		t.Fatalf("served %q, want real poster bytes", string(got))
	}
}

func TestIsPrivateHost(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.5", "192.168.1.10", "169.254.169.254", "0.0.0.0", "::1", ""}
	for _, h := range blocked {
		if !isPrivateHost(h) {
			t.Errorf("isPrivateHost(%q) = false, want true (literal private/loopback IP)", h)
		}
	}
	// Hostnames must NOT be blocked even though GFW DNS poisoning may resolve
	// them to private/loopback IPs — blocking them broke legitimate posters.
	allowed := []string{"image.tmdb.org", "lain.bgm.tv", "example.com", "8.8.8.8"}
	for _, h := range allowed {
		if isPrivateHost(h) {
			t.Errorf("isPrivateHost(%q) = true, want false", h)
		}
	}
}
