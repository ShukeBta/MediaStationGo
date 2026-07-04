package handler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

var handlerTestJPEG = []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0xff, 0xd9}

func TestCloudMountLibraryNameDefaultsToDirectoryBaseName(t *testing.T) {
	tests := []struct {
		name       string
		provider   string
		dir        string
		displayDir string
		want       string
	}{
		{name: "openlist directory", provider: "openlist", dir: "/国产剧", displayDir: "/国产剧", want: "国产剧"},
		{name: "nested directory", provider: "openlist", dir: "id-123", displayDir: "剧集/国产剧", want: "国产剧"},
		{name: "provider root", provider: "openlist", dir: "", displayDir: "", want: "OpenList"},
		{name: "115 root id", provider: "cloud115", dir: "0", displayDir: "", want: "115 网盘"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cloudMountLibraryName(tt.provider, tt.dir, tt.displayDir); got != tt.want {
				t.Fatalf("cloudMountLibraryName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCloudPlaybackDiagnosticsDoNotExposeRawRefOrURL(t *testing.T) {
	rawRef := "/剧集/国产剧/很长的敏感文件名.S01E01.mkv"
	refHash, refExt := cloudPlaybackRefFingerprint(rawRef)
	if refHash == "" || strings.Contains(rawRef, refHash) {
		t.Fatalf("ref hash should be a short fingerprint, got %q", refHash)
	}
	if refExt != ".mkv" {
		t.Fatalf("ref ext = %q, want .mkv", refExt)
	}
	if host := cloudPlaybackLinkHost("https://cdn.example.test/movie.mkv?token=secret"); host != "cdn.example.test" {
		t.Fatalf("host = %q, want cdn.example.test", host)
	}
	names := cloudPlaybackHeaderNames(map[string]string{
		"Authorization": "Bearer secret",
		"Cookie":        "sid=secret",
	})
	if got := strings.Join(names, ","); got != "Authorization,Cookie" {
		t.Fatalf("header names = %q", got)
	}
}

func TestAdminCloudHandlersRejectQuarkBrowsing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/admin/cloud/:type/list", cloudListHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/admin/cloud/quark/list?dir=0", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unsupported cloud provider") {
		t.Fatalf("body = %s, want unsupported cloud provider", w.Body.String())
	}
}

func TestCloudPlayRejectsQuarkProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/api/cloud/play/:type", cloudPlayHandler(nil))

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/play/quark?ref=file-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unsupported cloud provider") {
		t.Fatalf("body = %s, want unsupported cloud provider", w.Body.String())
	}
}

func TestCloudArtworkProxyServesCachedImageWithoutCloudResolve(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(handlerTestJPEG)
	}))
	defer upstream.Close()

	imageProxy := service.NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	stableKey := "openlist:/Anime/JianLai/poster.jpg"
	if err := imageProxy.PrefetchCloudResolved(t.Context(), stableKey, &cloud.DirectLink{URL: upstream.URL + "/poster.jpg"}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/api/img/cloud/:type", cloudArtworkProxyHandler(&service.Container{ImageProxy: imageProxy}))

	req := httptest.NewRequest(http.MethodGet, "/api/img/cloud/openlist?ref=%2FAnime%2FJianLai%2Fposter.jpg", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	if got := w.Body.Bytes(); !bytes.Equal(got, handlerTestJPEG) {
		t.Fatalf("body = %q, want cached poster", got)
	}
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "max-age=2592000") {
		t.Fatalf("cache-control = %q, want long static cache", got)
	}
}

func TestCloudArtworkProxyAcceptsCachedTBNImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(handlerTestJPEG)
	}))
	defer upstream.Close()

	imageProxy := service.NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: t.TempDir()}}, zap.NewNop())
	stableKey := "openlist:/Movies/Movie.tbn"
	if err := imageProxy.PrefetchCloudResolved(t.Context(), stableKey, &cloud.DirectLink{URL: upstream.URL + "/Movie.tbn"}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.GET("/api/img/cloud/:type", cloudArtworkProxyHandler(&service.Container{ImageProxy: imageProxy}))

	req := httptest.NewRequest(http.MethodGet, "/api/img/cloud/openlist?ref=%2FMovies%2FMovie.tbn", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	if got := w.Body.Bytes(); !bytes.Equal(got, handlerTestJPEG) {
		t.Fatalf("body = %q, want cached tbn poster", got)
	}
}
