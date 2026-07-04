package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestEffectiveVersionPrefersBuildVersion(t *testing.T) {
	t.Setenv("MEDIASTATION_VERSION", "MediaStationGo-v0.1.15")

	if got := effectiveVersion("MediaStationGo-v0.1.16"); got != "MediaStationGo-v0.1.16" {
		t.Fatalf("effectiveVersion = %q, want MediaStationGo-v0.1.16", got)
	}
}

func TestEffectiveVersionUsesEnvWhenBuildVersionIsDev(t *testing.T) {
	t.Setenv("MEDIASTATION_VERSION", " MediaStationGo-v0.1.16 ")

	if got := effectiveVersion("dev"); got != "MediaStationGo-v0.1.16" {
		t.Fatalf("effectiveVersion = %q, want MediaStationGo-v0.1.16", got)
	}
}

func TestEffectiveVersionDefaultsToDev(t *testing.T) {
	t.Setenv("MEDIASTATION_VERSION", "")

	if got := effectiveVersion(""); got != "dev" {
		t.Fatalf("effectiveVersion = %q, want dev", got)
	}
}

func TestServeSPANoCachesIndexAndServesRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	webDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<html><div id=\"root\"></div></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "favicon.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	serveSPA(router, webDir)

	for _, path := range []string{"/", "/login", "/library/e1c3507e-2878-40ae-a0e1-6b6e44b7fa7a", "/media/abc"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", path, w.Code)
		}
		if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
			t.Fatalf("%s Cache-Control = %q, want no-store", path, got)
		}
		if !strings.Contains(w.Body.String(), "root") {
			t.Fatalf("%s did not serve index.html: %q", path, w.Body.String())
		}
	}
}

func TestServeSPAServesAssetsImmutableAndBypassesAPIRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	webDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(webDir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(webDir, "brand"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("index"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "assets", "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "brand", "mediastationgo-logo.svg"), []byte("<svg></svg>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDir, "artwork-cache-sw.js"), []byte("self.addEventListener('fetch', () => {})"), 0o644); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	serveSPA(router, webDir)

	assetReq := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	assetResp := httptest.NewRecorder()
	router.ServeHTTP(assetResp, assetReq)
	if assetResp.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want 200", assetResp.Code)
	}
	if got := assetResp.Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
		t.Fatalf("asset Cache-Control = %q, want immutable", got)
	}

	brandReq := httptest.NewRequest(http.MethodGet, "/brand/mediastationgo-logo.svg", nil)
	brandResp := httptest.NewRecorder()
	router.ServeHTTP(brandResp, brandReq)
	if brandResp.Code != http.StatusOK {
		t.Fatalf("brand asset status = %d, want 200", brandResp.Code)
	}
	if got := brandResp.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("brand asset Cache-Control = %q, want no-store", got)
	}
	if strings.Contains(brandResp.Body.String(), "index") {
		t.Fatalf("brand asset should not serve SPA index: %q", brandResp.Body.String())
	}

	swReq := httptest.NewRequest(http.MethodGet, "/artwork-cache-sw.js", nil)
	swResp := httptest.NewRecorder()
	router.ServeHTTP(swResp, swReq)
	if swResp.Code != http.StatusOK {
		t.Fatalf("service worker status = %d, want 200", swResp.Code)
	}
	if got := swResp.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("service worker Cache-Control = %q, want no-store", got)
	}
	if strings.Contains(swResp.Body.String(), "index") {
		t.Fatalf("service worker should not serve SPA index: %q", swResp.Body.String())
	}

	for _, path := range []string{
		"/api/missing",
		"/emby",
		"/emby/missing",
		"/Library/VirtualFolders",
		"/Startup/Configuration",
		"/QuickConnect/Enabled",
		"/embywebsocket",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusNotFound {
			t.Fatalf("%s fallback status = %d, want 404", path, resp.Code)
		}
		if strings.Contains(resp.Body.String(), "index") {
			t.Fatalf("%s should not serve SPA index: %q", path, resp.Body.String())
		}
	}
}

func TestServeSPAMissingIndexReportsExplicit404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	serveSPA(router, t.TempDir())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
	if !strings.Contains(w.Body.String(), "web UI not found") {
		t.Fatalf("body = %q, want explicit missing UI message", w.Body.String())
	}
}
