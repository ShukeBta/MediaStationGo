// Package service — image proxy.
//
// Some deployments cannot reach image.tmdb.org directly (GFW, internal-only
// networks). ImageProxy fronts a remote image URL so the browser only ever
// talks to the MediaStationGo origin. The proxy:
//
//   - validates the URL scheme is http/https,
//   - streams bytes through with a small disk cache under cache/images,
//   - falls back to a transparent 1×1 PNG on upstream failure so the UI
//     never breaks layout,
//   - honors HTTP(S)_PROXY environment variables so users behind GFW can
//     route image fetches through their proxy.
package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// transparent1x1PNG is a baseline 67-byte PNG used as a fallback when the
// upstream image cannot be retrieved, so browser layouts never collapse.
var transparent1x1PNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

// knownImageHosts are hosts we explicitly recognize. The list is no longer
// a hard allow-list — it only short-circuits cases where we can be 100%
// sure the destination is a public image CDN. Other hosts are accepted as
// long as the scheme is http/https; this is required so users behind GFW
// can configure their own TMDb mirror via secrets.tmdb_image_proxy.
var knownImageHosts = map[string]struct{}{
	"image.tmdb.org":       {},
	"www.themoviedb.org":   {},
	"lain.bgm.tv":          {},
	"img.bgm.tv":           {},
	"webdav.bgm.tv":        {},
	"img1.doubanio.com":    {},
	"img2.doubanio.com":    {},
	"img3.doubanio.com":    {},
	"img9.doubanio.com":    {},
	"assets.fanart.tv":     {},
	"artworks.thetvdb.com": {},
}

// ImageProxy fetches and caches remote images on behalf of the browser.
type ImageProxy struct {
	cfg      *config.Config
	log      *zap.Logger
	client   *http.Client
	cacheDir string
	mu       sync.Mutex
}

// NewImageProxy is the constructor.
func NewImageProxy(cfg *config.Config, log *zap.Logger) *ImageProxy {
	// Honor HTTP(S)_PROXY env vars so deployments behind GFW can pull
	// from image.tmdb.org via their HTTP proxy without extra config. On
	// Windows we also honor the current user's system proxy settings.
	transport := NewExternalTransport()
	return &ImageProxy{
		cfg:      cfg,
		log:      log,
		cacheDir: filepath.Join(cfg.Cache.CacheDir, "images"),
		client:   &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

// validateURL parses raw and ensures the scheme is http/https and the
// target host is not a private/loopback/link-local address (SSRF guard).
func (p *ImageProxy) validateURL(raw string) (*url.URL, error) {
	if raw == "" {
		return nil, errors.New("missing url")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, errors.New("invalid url")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, errors.New("unsupported scheme")
	}
	if isPrivateHost(u.Hostname()) {
		return nil, errors.New("requests to private/internal hosts are not allowed")
	}
	return u, nil
}

// isPrivateHost returns true if host resolves to a loopback, private, or
// link-local address. This blocks SSRF attacks that try to reach internal
// services (e.g. cloud metadata at 169.254.169.254).
func isPrivateHost(host string) bool {
	if host == "" {
		return true
	}
	ip := net.ParseIP(host)
	if ip != nil {
		return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
	}
	addrs, err := net.LookupHost(host)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		resolved := net.ParseIP(addr)
		if resolved != nil && (resolved.IsLoopback() || resolved.IsPrivate() || resolved.IsLinkLocalUnicast() || resolved.IsLinkLocalMulticast() || resolved.IsUnspecified()) {
			return true
		}
	}
	return false
}

// isAllowedLocalPath restricts local file reads to the data directory and
// cache directory to prevent arbitrary file read via path traversal.
func (p *ImageProxy) isAllowedLocalPath(abs string) bool {
	for _, root := range []string{p.cfg.App.DataDir, p.cfg.Cache.CacheDir, p.cfg.Media.MoviesDir, p.cfg.Media.TVDir, p.cfg.Media.AnimeDir} {
		if root == "" {
			continue
		}
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		if strings.HasPrefix(abs, rootAbs+string(filepath.Separator)) || abs == rootAbs {
			return true
		}
	}
	return false
}

func isLocalImagePath(raw string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" || isHTTPish(raw) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(raw))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".bmp":
		return true
	default:
		return false
	}
}

func isHTTPish(raw string) bool {
	return strings.HasPrefix(strings.ToLower(raw), "http://") || strings.HasPrefix(strings.ToLower(raw), "https://")
}

// detectContentType returns the MIME type of data using the first 512 bytes.
func detectContentType(data []byte) string {
	if len(data) > 512 {
		return http.DetectContentType(data[:512])
	}
	return http.DetectContentType(data)
}

// servePlaceholder writes a 1×1 transparent PNG to w. Used as a fallback
// when upstream fetch fails so the browser layout stays intact.
func servePlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(transparent1x1PNG)
}

// Serve writes the requested image to w. Caller is expected to validate
// the JWT before invoking it.
func (p *ImageProxy) Serve(ctx context.Context, w http.ResponseWriter, r *http.Request, raw string) error {
	if isLocalImagePath(raw) {
		path := filepath.Clean(raw)
		abs, err := filepath.Abs(path)
		if err != nil || !p.isAllowedLocalPath(abs) {
			servePlaceholder(w)
			return nil
		}
		path = abs
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			servePlaceholder(w)
			return nil
		}
		stat, _ := os.Stat(path)
		modTime := time.Now()
		if stat != nil {
			modTime = stat.ModTime()
		}
		w.Header().Set("Content-Type", detectContentType(data))
		w.Header().Set("Cache-Control", "public, max-age=604800")
		http.ServeContent(w, r, filepath.Base(path), modTime, bytes.NewReader(data))
		return nil
	}

	u, err := p.validateURL(raw)
	if err != nil {
		// Bad URL is the only request-side error; everything else falls
		// through to the placeholder so the UI stays clean.
		return err
	}
	host := strings.ToLower(u.Host)

	// Cache key = sha1(url)
	sum := sha1.Sum([]byte(raw))
	key := hex.EncodeToString(sum[:])
	cachePath := filepath.Join(p.cacheDir, key)

	// Cache hit.
	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		w.Header().Set("Content-Type", detectContentType(data))
		w.Header().Set("Cache-Control", "public, max-age=604800")
		stat, _ := os.Stat(cachePath)
		modTime := time.Now()
		if stat != nil {
			modTime = stat.ModTime()
		}
		http.ServeContent(w, r, key, modTime, bytes.NewReader(data))
		return nil
	}

	// Cache miss → fetch upstream.
	if err := os.MkdirAll(p.cacheDir, 0o755); err != nil {
		p.log.Warn("imageproxy: mkdir failed", zap.String("dir", p.cacheDir), zap.Error(err))
		servePlaceholder(w)
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		p.log.Warn("imageproxy: build request failed", zap.String("url", raw), zap.Error(err))
		servePlaceholder(w)
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0 Safari/537.36")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
	if strings.Contains(host, "doubanio.com") {
		req.Header.Set("Referer", "https://movie.douban.com/")
	}

	resp, err := p.client.Do(req)
	if err != nil {
		p.log.Warn("imageproxy: upstream fetch failed",
			zap.String("host", host), zap.Error(err))
		servePlaceholder(w)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		p.log.Warn("imageproxy: upstream returned non-OK",
			zap.String("host", host), zap.String("status", resp.Status))
		servePlaceholder(w)
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MiB cap
	if err != nil || len(data) == 0 {
		p.log.Warn("imageproxy: read upstream body failed",
			zap.String("host", host), zap.Error(err))
		servePlaceholder(w)
		return nil
	}

	// Write to a temp file then rename for atomicity.
	p.mu.Lock()
	tmp, tmpErr := os.CreateTemp(p.cacheDir, "img-*.tmp")
	if tmpErr == nil {
		if _, werr := tmp.Write(data); werr == nil {
			tmp.Close()
			if rerr := os.Rename(tmp.Name(), cachePath); rerr != nil {
				_ = os.Remove(tmp.Name())
			}
		} else {
			tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}
	p.mu.Unlock()

	ctype := resp.Header.Get("Content-Type")
	if ctype == "" {
		ctype = detectContentType(data)
	}
	w.Header().Set("Content-Type", ctype)
	if v := resp.Header.Get("Content-Length"); v != "" {
		w.Header().Set("Content-Length", v)
	}
	if v := resp.Header.Get("ETag"); v != "" {
		w.Header().Set("ETag", v)
	}
	if v := resp.Header.Get("Last-Modified"); v != "" {
		w.Header().Set("Last-Modified", v)
	}
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeContent(w, r, key, time.Now(), bytes.NewReader(data))
	return nil
}

// Fetch 拉取远程图片并返回字节和 Content-Type（带缓存）。
func (p *ImageProxy) Fetch(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := p.validateURL(raw)
	if err != nil {
		return nil, "", err
	}

	// Cache lookup
	sum := sha1.Sum([]byte(raw))
	key := hex.EncodeToString(sum[:])
	cachePath := filepath.Join(p.cacheDir, key)

	if data, err := os.ReadFile(cachePath); err == nil && len(data) > 0 {
		return data, detectContentType(data), nil
	}

	// Fetch upstream
	if err := os.MkdirAll(p.cacheDir, 0o755); err != nil {
		return nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "MediaStationGo/0.1")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", errors.New("upstream returned " + resp.Status)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, "", err
	}

	// Write to cache
	p.mu.Lock()
	tmp, terr := os.CreateTemp(p.cacheDir, "img-*.tmp")
	if terr == nil {
		if _, werr := tmp.Write(data); werr == nil {
			tmp.Close()
			if rerr := os.Rename(tmp.Name(), cachePath); rerr != nil {
				_ = os.Remove(tmp.Name())
			}
		} else {
			tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}
	p.mu.Unlock()

	ctype := resp.Header.Get("Content-Type")
	if ctype == "" {
		ctype = detectContentType(data)
	}
	// host is unused here but referenced for log clarity in the future.
	_ = u
	return data, ctype, nil
}
