package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestImageProxyCachesFailedRemoteImageFetch(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
			Request:    req,
		}, nil
	})}
	raw := "https://image.tmdb.org/t/p/w500/poster.jpg"
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), raw); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if rec.Body.Len() != len(transparent1x1PNG) {
			t.Fatalf("body length = %d, want placeholder %d", rec.Body.Len(), len(transparent1x1PNG))
		}
		if got := rec.Header().Get("Cache-Control"); got != imagePlaceholderCacheControl {
			t.Fatalf("Cache-Control = %q, want %q", got, imagePlaceholderCacheControl)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1 due to negative cache", got)
	}
}

func TestImageProxyRemoveFailedAllowsRetry(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		call := atomic.AddInt32(&calls, 1)
		if call == 1 {
			return &http.Response{
				StatusCode: http.StatusBadGateway,
				Status:     "502 Bad Gateway",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
				Request:    req,
			}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(testJPEG)),
			Request:    req,
		}, nil
	})}

	raw := "https://image.tmdb.org/t/p/w500/retry-poster.jpg"
	rec := httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), raw); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != len(transparent1x1PNG) {
		t.Fatalf("first body length = %d, want placeholder %d", rec.Body.Len(), len(transparent1x1PNG))
	}
	if err := proxy.RemoveFailed(raw); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img?v=retry", nil), raw); err != nil {
		t.Fatal(err)
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("retried body = %x, want poster bytes", got)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2 after retry", got)
	}
}

func TestImageProxyPrefetchRemoteUsesProviderHeaders(t *testing.T) {
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Referer"); got != "https://movie.douban.com/" {
			t.Fatalf("Referer = %q, want Douban movie referer", got)
		}
		if got := req.Header.Get("User-Agent"); !strings.Contains(got, "Mozilla/5.0") {
			t.Fatalf("User-Agent = %q, want browser-like UA", got)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
			Body:       io.NopCloser(bytes.NewReader(testJPEG)),
			Request:    req,
		}, nil
	})}

	raw := "https://img9.doubanio.com/view/photo/s_ratio_poster/public/p2933012346.jpg"
	if err := proxy.PrefetchRemote(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
}

func TestImageProxyRemoveCachedAllowsRefresh(t *testing.T) {
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

	raw := "https://image.tmdb.org/t/p/w500/refresh-poster.jpg"
	if err := proxy.PrefetchRemote(t.Context(), raw); err != nil {
		t.Fatal(err)
	}
	if err := proxy.RemoveCached(raw); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls before refresh = %d, want 1", got)
	}
	rec := httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img?refresh=1", nil), raw); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("upstream calls after refresh = %d, want 2", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestImageProxyRefreshKeepsCachedImageOnUpstreamFailure(t *testing.T) {
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
			Request:    req,
		}, nil
	})}

	raw := "https://image.tmdb.org/t/p/w500/cached-poster.jpg"
	_, cachePath, _, err := proxy.remoteImageCachePaths(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		t.Fatal(err)
	}
	cachedPoster := testJPEG
	if err := os.WriteFile(cachePath, cachedPoster, 0o600); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img?refresh=1", nil), raw); err != nil {
		t.Fatal(err)
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, cachedPoster) {
		t.Fatalf("body = %x, want cached poster after failed refresh", got)
	}
}

func TestImageProxyRefetchesTransparentPlaceholderCache(t *testing.T) {
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

	raw := "https://img1.doubanio.com/view/photo/s_ratio_poster/public/p2925358079.jpg"
	_, cachePath, failPath, err := proxy.remoteImageCachePaths(raw)
	if err != nil {
		t.Fatal(err)
	}
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
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img?retry=1", nil), raw); err != nil {
		t.Fatal(err)
	}
	if got := rec.Body.Bytes(); !bytes.Equal(got, testJPEG) {
		t.Fatalf("body = %x, want refetched poster bytes", got)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestImageProxyDoesNotCacheNonImageRemoteResponse(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"text/html"}},
			Body:       io.NopCloser(strings.NewReader("<html>not image</html>")),
			Request:    req,
		}, nil
	})}

	raw := "https://img1.doubanio.com/view/photo/s_ratio_poster/public/p-bad.jpg"
	rec := httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), raw); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != len(transparent1x1PNG) {
		t.Fatalf("body length = %d, want placeholder %d", rec.Body.Len(), len(transparent1x1PNG))
	}
	_, cachePath, _, err := proxy.remoteImageCachePaths(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("non-image response should not be cached, stat err=%v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestImageProxyDoesNotCacheMislabeledRemoteResponse(t *testing.T) {
	var calls int32
	proxy := NewImageProxy(&config.Config{Cache: config.CacheConfig{CacheDir: filepath.Join(t.TempDir(), "cache")}}, zap.NewNop())
	proxy.client = &http.Client{Transport: imageRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&calls, 1)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header:     http.Header{"Content-Type": []string{"image/jpeg"}},
			Body:       io.NopCloser(strings.NewReader("<html>not a poster</html>")),
			Request:    req,
		}, nil
	})}

	raw := "https://img1.doubanio.com/view/photo/s_ratio_poster/public/p-mislabeled.jpg"
	rec := httptest.NewRecorder()
	if err := proxy.Serve(t.Context(), rec, httptest.NewRequest(http.MethodGet, "/api/img", nil), raw); err != nil {
		t.Fatal(err)
	}
	if rec.Body.Len() != len(transparent1x1PNG) {
		t.Fatalf("body length = %d, want placeholder %d", rec.Body.Len(), len(transparent1x1PNG))
	}
	_, cachePath, _, err := proxy.remoteImageCachePaths(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cachePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("mislabeled non-image response should not be cached, stat err=%v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}
