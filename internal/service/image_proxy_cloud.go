package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// CloudImageCached reports whether a stable cloud-image ref already has a
// usable positive or short-lived negative cache entry. Scanner pre-warm uses it
// to avoid repeatedly resolving the same cloud sidecar image.
func (p *ImageProxy) CloudImageCached(stableKey string) bool {
	if p == nil {
		return false
	}
	_, cachePath, failPath := p.cloudImageCachePaths(stableKey)
	if data, err := os.ReadFile(cachePath); err == nil {
		if _, ok := validImageContentType(data); ok {
			return true
		}
		_ = os.Remove(cachePath)
		_ = os.Remove(failPath)
	}
	return freshNegativeImageCache(failPath)
}

// ServeCloudCached serves an already-local cloud sidecar image without asking
// the cloud provider for a fresh direct link. It returns true when it wrote a
// response, including a fresh negative-cache placeholder.
func (p *ImageProxy) ServeCloudCached(w http.ResponseWriter, r *http.Request, stableKey string) bool {
	if p == nil {
		return false
	}
	key, cachePath, failPath := p.cloudImageCachePaths(stableKey)
	p.removeUnusableImageCache(cachePath, failPath)
	if serveCachedImageFile(w, r, key, cachePath) {
		return true
	}
	if freshNegativeImageCache(failPath) {
		serveCachedPlaceholder(w)
		return true
	}
	return false
}

// ServeCloudResolved stores a cloud sidecar image in the same disk cache used
// by remote posters, then serves it with long browser-cache headers.
func (p *ImageProxy) ServeCloudResolved(ctx context.Context, w http.ResponseWriter, r *http.Request, stableKey string, link *cloud.DirectLink) error {
	if p == nil || link == nil || strings.TrimSpace(link.URL) == "" {
		servePlaceholder(w)
		return nil
	}
	stableKey = strings.TrimSpace(stableKey)
	if stableKey == "" {
		stableKey = link.URL
	}
	key, cachePath, failPath := p.cloudImageCachePaths(stableKey)
	p.removeUnusableImageCache(cachePath, failPath)
	if serveCachedImageFile(w, r, key, cachePath) {
		return nil
	}
	if freshNegativeImageCache(failPath) {
		serveCachedPlaceholder(w)
		return nil
	}
	data, ctype, err := p.fetchAndCacheCloudImage(ctx, stableKey, link, r.UserAgent())
	if err != nil {
		p.log.Warn("imageproxy: cloud image fetch failed", zap.String("url", link.URL), zap.Error(err))
		serveCachedPlaceholder(w)
		return nil
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", imageBrowserCacheControl)
	modTime := time.Now()
	if stat, err := os.Stat(cachePath); err == nil && stat.Size() > 0 {
		modTime = stat.ModTime()
		w.Header().Set("ETag", imageFileETag(key, stat))
	}
	http.ServeContent(w, r, key, modTime, bytes.NewReader(data))
	return nil
}

// PrefetchCloudResolved downloads a cloud sidecar image into the local cache
// without writing an HTTP response.
func (p *ImageProxy) PrefetchCloudResolved(ctx context.Context, stableKey string, link *cloud.DirectLink) error {
	if p == nil || link == nil || strings.TrimSpace(link.URL) == "" {
		return nil
	}
	if p.CloudImageCached(stableKey) {
		return nil
	}
	_, _, err := p.fetchAndCacheCloudImage(ctx, stableKey, link, "MediaStationGo/0.1")
	return err
}

func (p *ImageProxy) fetchAndCacheCloudImage(ctx context.Context, stableKey string, link *cloud.DirectLink, userAgent string) ([]byte, string, error) {
	if err := os.MkdirAll(p.cacheDir, 0o750); err != nil {
		return nil, "", err
	}
	_, cachePath, failPath := p.cloudImageCachePaths(stableKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link.URL, nil)
	if err != nil {
		return nil, "", err
	}
	for k, v := range link.Headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		if strings.TrimSpace(userAgent) != "" {
			req.Header.Set("User-Agent", userAgent)
		} else {
			req.Header.Set("User-Agent", "MediaStationGo/0.1")
		}
	}
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	resp, err := p.client.Do(req)
	if err != nil {
		p.markImageFetchFailed(failPath)
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		p.markImageFetchFailed(failPath)
		return nil, "", errors.New("cloud image returned " + resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		p.markImageFetchFailed(failPath)
		return nil, "", err
	}
	if len(data) == 0 {
		p.markImageFetchFailed(failPath)
		return nil, "", errors.New("cloud image body is empty")
	}
	ctype, ok := validImageContentType(data)
	if !ok {
		p.markImageFetchFailed(failPath)
		return nil, "", errors.New("cloud image returned non-image content")
	}
	p.writeImageCache(cachePath, failPath, "img-cloud-*.tmp", data)
	return data, ctype, nil
}
