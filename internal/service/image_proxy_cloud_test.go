package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func TestImageProxyCachesCloudResolvedImage(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(testJPEG)),
			Request:    req,
		}, nil
	})}

	link := &cloud.DirectLink{URL: "http://cloud-provider.invalid/poster.png"}
	if proxy.ServeCloudCached(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=poster.png", nil), "openlist:poster.png") {
		t.Fatal("ServeCloudCached returned true before the cloud image was cached")
	}
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		if err := proxy.ServeCloudResolved(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=poster.png", nil), "openlist:poster.png", link); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != imageBrowserCacheControl {
			t.Fatalf("Cache-Control = %q, want %q", got, imageBrowserCacheControl)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 due to cloud image cache", got)
	}

	rec := httptest.NewRecorder()
	if !proxy.ServeCloudCached(rec, httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=poster.png", nil), "openlist:poster.png") {
		t.Fatal("ServeCloudCached returned false after the cloud image was cached")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls after ServeCloudCached = %d, want 1", got)
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("cached body = %x, want cached cloud image", got)
	}
}

func TestImageProxyPrefetchCloudResolvedImage(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(testJPEG)),
			Request:    req,
		}, nil
	})}

	link := &cloud.DirectLink{URL: "http://cloud-provider.invalid/folder.png"}
	if err := proxy.PrefetchCloudResolved(t.Context(), "openlist:folder.png", link); err != nil {
		t.Fatal(err)
	}
	if err := proxy.PrefetchCloudResolved(t.Context(), "openlist:folder.png", link); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 after prefetch cache hit", got)
	}
	rec := httptest.NewRecorder()
	if !proxy.ServeCloudCached(rec, httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=folder.png", nil), "openlist:folder.png") {
		t.Fatal("prefetched cloud image was not served from cache")
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("cached body = %x, want prefetched cloud image", got)
	}
}

func TestImageProxyPrefetchCloudResolvedRefetchesInvalidCache(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(testJPEG)),
			Request:    req,
		}, nil
	})}

	stableKey := "openlist:bad-cache-poster.jpg"
	_, cachePath, failPath := proxy.cloudImageCachePaths(stableKey)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, []byte("<html>old bad cache</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failPath, []byte("failed"), 0o600); err != nil {
		t.Fatal(err)
	}

	link := &cloud.DirectLink{URL: "http://cloud-provider.invalid/bad-cache-poster.jpg"}
	if err := proxy.PrefetchCloudResolved(t.Context(), stableKey, link); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 after invalid cache cleanup", got)
	}
	rec := httptest.NewRecorder()
	if !proxy.ServeCloudCached(rec, httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=bad-cache-poster.jpg", nil), stableKey) {
		t.Fatal("refetched cloud image was not served from cache")
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("cached body = %x, want refetched cloud image", got)
	}
}

func TestImageProxyServeCloudCachedSkipsInvalidCache(t *testing.T) {
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	stableKey := "openlist:invalid-cached-poster.jpg"
	_, cachePath, failPath := proxy.cloudImageCachePaths(stableKey)
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cachePath, transparent1x1PNG, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failPath, []byte("failed"), 0o600); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	if proxy.ServeCloudCached(rec, httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=invalid-cached-poster.jpg", nil), stableKey) {
		t.Fatal("ServeCloudCached should skip invalid cached cloud artwork")
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid cache should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(failPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale fail marker should be removed with invalid cache, stat err=%v", err)
	}
}
