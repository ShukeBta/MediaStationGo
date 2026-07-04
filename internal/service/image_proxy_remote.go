package service

import (
	"bytes"
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
	_, cachePath, failPath, err := p.remoteImageCachePaths(raw)
	if err != nil {
		return nil
	}
	if err := os.Remove(cachePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.Remove(failPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
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
	if !serveImageFile(w, r, filepath.Base(abs), abs, imageBrowserCacheControl) {
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
	p.removeUnusableImageCache(cachePath, failPath)
	if !forceRefresh && serveCachedImageFile(w, r, key, cachePath) {
		return nil
	}
	if !forceRefresh && p.serveFreshRemoteFailure(w, failPath) {
		return nil
	}
	data, ctype, contentLength, err := p.fetchAndCacheRemoteImage(ctx, raw, host, cachePath, failPath)
	if err != nil {
		if forceRefresh && serveCachedImageFile(w, r, key, cachePath) {
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
	w.Header().Set("Cache-Control", imageBrowserCacheControl)
	http.ServeContent(w, r, key, modTime, bytes.NewReader(data))
	return nil
}

func (p *ImageProxy) removeUnusableImageCache(cachePath, failPath string) {
	data, err := os.ReadFile(cachePath) // #nosec G304 -- cachePath is SHA-derived under cacheDir.
	if err != nil {
		return
	}
	ctype := detectContentType(data)
	if len(data) > 0 && isImageContentType(ctype) && !isTransparentPlaceholderData(data) {
		return
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
			p.writeImageCache(cachePath, failPath, "img-*.tmp", data)
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
			p.writeImageCache(cachePath, failPath, "img-*.tmp", data)
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
	u, err := p.validateURL(raw)
	if err != nil {
		return nil, "", err
	}
	host := strings.ToLower(u.Host)
	_, cachePath, failPath := p.remoteImageCachePathsForValidated(raw)
	p.removeUnusableImageCache(cachePath, failPath)
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 { // #nosec G304 -- cachePath is SHA-derived under cacheDir.
		ctype := detectContentType(data)
		if isImageContentType(ctype) && !isTransparentPlaceholderData(data) {
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

func (p *ImageProxy) writeImageCache(cachePath, failPath, pattern string, data []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	tmp, tmpErr := os.CreateTemp(p.cacheDir, pattern)
	if tmpErr != nil {
		return
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return
	}
	_ = tmp.Close()
	if err := os.Rename(tmp.Name(), cachePath); err != nil {
		_ = os.Remove(tmp.Name())
		return
	}
	_ = os.Remove(failPath)
}
