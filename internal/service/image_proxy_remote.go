package service

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

var errImageProxyRequestSetup = errors.New("image proxy request setup failed")
var errImageProxyNonImageContent = errors.New("upstream returned non-image content")

func (p *ImageProxy) PrefetchRemote(ctx context.Context, raw string) error {
	_, _, err := p.Fetch(ctx, raw)
	return err
}

func (p *ImageProxy) RemoveCached(raw string) error {
	if !isHTTPish(raw) {
		return nil
	}
	key, cachePath, failPath, err := p.remoteImageCachePaths(raw)
	if err != nil {
		return nil
	}
	if err := os.Remove(cachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(failPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return p.removeImageVariantCache(key)
}

func (p *ImageProxy) RemoveFailed(raw string) error {
	if !isHTTPish(raw) {
		return nil
	}
	_, _, failPath, err := p.remoteImageCachePaths(raw)
	if err != nil {
		return nil
	}
	if err := os.Remove(failPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Serve writes the requested image to w. Caller is expected to validate
// the JWT before invoking it.
func (p *ImageProxy) Serve(ctx context.Context, w http.ResponseWriter, r *http.Request, raw string) error {
	if isLocalImagePath(raw) {
		return p.serveLocalImage(w, r, raw)
	}
	return p.serveRemoteImage(ctx, w, r, raw)
}

func (p *ImageProxy) serveLocalImage(w http.ResponseWriter, r *http.Request, raw string) error {
	path := filepath.Clean(raw)
	abs, err := filepath.Abs(path)
	if err != nil || !p.isAllowedLocalPath(abs) {
		servePlaceholder(w)
		return nil
	}
	if !p.serveImageFile(w, r, "local:"+abs, abs, imageBrowserCacheControl) {
		servePlaceholder(w)
	}
	return nil
}

func (p *ImageProxy) serveRemoteImage(ctx context.Context, w http.ResponseWriter, r *http.Request, raw string) error {
	u, err := p.validateURL(raw)
	if err != nil {
		return err
	}
	host := strings.ToLower(u.Host)
	key, cachePath, failPath := p.remoteImageCachePathsForValidated(raw)
	forceRefresh := r.URL.Query().Get("refresh") != ""
	p.removeUnusableRemoteImageCache(host, cachePath, failPath)
	if !forceRefresh && p.serveCachedImageFile(w, r, key, cachePath) {
		return nil
	}
	if !forceRefresh && p.serveFreshRemoteFailure(w, failPath) {
		return nil
	}
	data, ctype, contentLength, err := p.fetchAndCacheRemoteImage(ctx, raw, host, cachePath, failPath)
	if err != nil {
		if forceRefresh && p.serveCachedImageFile(w, r, key, cachePath) {
			return nil
		}
		if errors.Is(err, errImageProxyRequestSetup) {
			servePlaceholder(w)
		} else {
			serveCachedPlaceholder(w)
		}
		return nil
	}
	w.Header().Set("Content-Type", ctype)
	if contentLength != "" {
		w.Header().Set("Content-Length", contentLength)
	}
	modTime := time.Now()
	if stat, err := os.Stat(cachePath); err == nil && stat.Size() > 0 {
		modTime = stat.ModTime()
		w.Header().Set("ETag", imageFileETag(key, stat))
	}
	p.serveImageBytes(w, r, key, modTime, data, ctype, contentLength, imageBrowserCacheControl)
	return nil
}

func (p *ImageProxy) removeUnusableImageCache(cachePath, failPath string) {
	p.removeUnusableRemoteImageCache("", cachePath, failPath)
}

func (p *ImageProxy) removeUnusableRemoteImageCache(host, cachePath, failPath string) {
	data, err := os.ReadFile(cachePath) // #nosec G304 -- cachePath is SHA-derived under cacheDir.
	if err != nil {
		return
	}
	if len(data) > 0 {
		if _, ok := validRemoteImageContentType(host, data); ok {
			return
		}
	}
	_ = os.Remove(cachePath)
	_ = os.Remove(failPath)
}

func (p *ImageProxy) serveFreshRemoteFailure(w http.ResponseWriter, failPath string) bool {
	if stat, err := os.Stat(failPath); err == nil && time.Since(stat.ModTime()) < imageNegativeCacheTTL {
		serveCachedPlaceholder(w)
		return true
	} else if err == nil {
		_ = os.Remove(failPath)
	}
	return false
}

func (p *ImageProxy) fetchAndCacheRemoteImage(ctx context.Context, raw, host, cachePath, failPath string) ([]byte, string, string, error) {
	if err := os.MkdirAll(p.cacheDir, 0o750); err != nil {
		p.log.Warn("imageproxy: mkdir failed", zap.String("dir", p.cacheDir), zap.Error(err))
		return nil, "", "", errImageProxyRequestSetup
	}
	var lastErr error
	for _, candidate := range p.remoteImageFetchClients() {
		data, ctype, contentLength, err := p.fetchRemoteImageOnce(ctx, raw, host, candidate)
		if err == nil {
			p.writeOriginalImageCache(cachePath, failPath, "img-*.tmp", data)
			return data, ctype, contentLength, nil
		}
		if errors.Is(err, errImageProxyRequestSetup) {
			return nil, "", "", err
		}
		lastErr = err
	}
	if p.canUseExternalImageFallback() && isDoubanImageHost(host) {
		data, ctype, contentLength, err := fetchRemoteImageWithCurl(ctx, raw, host)
		if err == nil {
			p.writeOriginalImageCache(cachePath, failPath, "img-*.tmp", data)
			return data, ctype, contentLength, nil
		}
		p.log.Warn("imageproxy: curl fallback failed", zap.String("host", host), zap.Error(err))
		lastErr = err
	}
	p.markImageFetchFailed(failPath)
	if lastErr == nil {
		lastErr = errors.New("upstream image fetch failed")
	}
	return nil, "", "", lastErr
}

// Fetch pulls a remote image and returns bytes plus Content-Type using cache.
func (p *ImageProxy) Fetch(ctx context.Context, raw string) ([]byte, string, error) {
	if isLocalImagePath(raw) {
		return p.fetchLocalImage(raw)
	}
	u, err := p.validateURL(raw)
	if err != nil {
		return nil, "", err
	}
	host := strings.ToLower(u.Host)
	_, cachePath, failPath := p.remoteImageCachePathsForValidated(raw)
	p.removeUnusableRemoteImageCache(host, cachePath, failPath)
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 { // #nosec G304 -- cachePath is SHA-derived under cacheDir.
		if ctype, ok := validRemoteImageContentType(host, data); ok {
			return data, ctype, nil
		}
		_ = os.Remove(cachePath)
		_ = os.Remove(failPath)
	}
	if stat, err := os.Stat(failPath); err == nil && time.Since(stat.ModTime()) < imageNegativeCacheTTL {
		return nil, "", errors.New("recent image fetch failure")
	} else if err == nil {
		_ = os.Remove(failPath)
	}
	data, ctype, _, err := p.fetchAndCacheRemoteImage(ctx, raw, host, cachePath, failPath)
	return data, ctype, err
}

func (p *ImageProxy) fetchLocalImage(raw string) ([]byte, string, error) {
	path := filepath.Clean(raw)
	abs, err := filepath.Abs(path)
	if err != nil || !p.isAllowedLocalPath(abs) {
		return nil, "", errors.New("local image path is not allowed")
	}
	data, err := os.ReadFile(abs) // #nosec G304 -- abs is constrained to configured media/cache roots by isAllowedLocalPath.
	if err != nil {
		return nil, "", err
	}
	ctype := detectContentType(data)
	if !isImageContentType(ctype) || isTransparentPlaceholderData(data) {
		return nil, "", errors.New("local image is not usable")
	}
	return data, ctype, nil
}

func (p *ImageProxy) writeOriginalImageCache(cachePath, failPath, pattern string, data []byte) bool {
	p.mu.Lock()
	written, previousSize := p.writeImageCacheLocked(cachePath, failPath, pattern, data)
	if written {
		p.noteImageCacheWrite(previousSize, int64(len(data)))
	}
	p.mu.Unlock()
	return written
}

func (p *ImageProxy) writeImageCache(cachePath, failPath, pattern string, data []byte) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	written, _ := p.writeImageCacheLocked(cachePath, failPath, pattern, data)
	return written
}

func (p *ImageProxy) writeImageCacheLocked(cachePath, failPath, pattern string, data []byte) (bool, int64) {
	var previousSize int64
	if stat, err := os.Stat(cachePath); err == nil && stat.Mode().IsRegular() {
		previousSize = stat.Size()
	}
	tmp, tmpErr := os.CreateTemp(p.cacheDir, pattern)
	if tmpErr != nil {
		return false, previousSize
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return false, previousSize
	}
	_ = tmp.Close()
	if err := os.Rename(tmp.Name(), cachePath); err != nil {
		_ = os.Remove(tmp.Name())
		return false, previousSize
	}
	if strings.TrimSpace(failPath) != "" {
		_ = os.Remove(failPath)
	}
	return true, previousSize
}

func (p *ImageProxy) writeImageVariantCache(cachePath string, data []byte) {
	if len(data) == 0 || len(data) > imageVariantCacheFileMaxBytes {
		return
	}
	p.variantCacheMu.Lock()
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		p.variantCacheMu.Unlock()
		if p.log != nil {
			p.log.Warn("imageproxy: variant cache mkdir failed", zap.String("dir", filepath.Dir(cachePath)), zap.Error(err))
		}
		return
	}

	currentSize := int64(0)
	if stat, err := os.Stat(cachePath); err == nil && stat.Mode().IsRegular() {
		currentSize = stat.Size()
	}
	if p.variantCacheBytes-currentSize+int64(len(data)) > imageVariantCacheMaxBytes {
		p.variantCacheMu.Unlock()
		p.scheduleImageVariantCachePrune(true)
		return
	}
	written := p.writeImageCache(cachePath, "", "img-variant-*.tmp", data)
	if written {
		p.variantCacheBytes += int64(len(data)) - currentSize
	}
	p.variantCacheMu.Unlock()
	if written {
		p.scheduleImageVariantCachePrune(false)
	}
}
