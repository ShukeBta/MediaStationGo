package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	_ "golang.org/x/image/webp"
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

// detectContentType returns the MIME type of data using the first 512 bytes.
func detectContentType(data []byte) string {
	if len(data) > 512 {
		return http.DetectContentType(data[:512])
	}
	return http.DetectContentType(data)
}

func isImageContentType(ctype string) bool {
	ctype = strings.ToLower(strings.TrimSpace(strings.Split(ctype, ";")[0]))
	return strings.HasPrefix(ctype, "image/")
}

func validImageContentType(data []byte) (string, bool) {
	detected := detectContentType(data)
	if isImageContentType(detected) && !isTransparentPlaceholderData(data) {
		return detected, true
	}
	return "", false
}

func isTransparentPlaceholderData(data []byte) bool {
	return bytes.Equal(data, transparent1x1PNG)
}

func validRemoteImageContentType(host string, data []byte) (string, bool) {
	ctype, ok := validImageContentType(data)
	if !ok || isRemoteImagePlaceholderData(host, data) {
		return "", false
	}
	return ctype, true
}

func isRemoteImagePlaceholderData(host string, data []byte) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	if host != "xximgs.cc" && !strings.HasSuffix(host, ".xximgs.cc") {
		return false
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return false
	}
	const samples = 17
	for yi := 0; yi < samples; yi++ {
		y := bounds.Min.Y
		if bounds.Dy() > 1 {
			y += yi * (bounds.Dy() - 1) / (samples - 1)
		}
		for xi := 0; xi < samples; xi++ {
			x := bounds.Min.X
			if bounds.Dx() > 1 {
				x += xi * (bounds.Dx() - 1) / (samples - 1)
			}
			r, g, b, a := img.At(x, y).RGBA()
			if a < 0xff00 || r < 0xf800 || g < 0xf800 || b < 0xf800 {
				return false
			}
		}
	}
	return true
}

// servePlaceholder writes a 1x1 transparent PNG to w. Used as a fallback
// when upstream fetch fails so the browser layout stays intact.
func servePlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(transparent1x1PNG)
}

func serveCachedPlaceholder(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", imagePlaceholderCacheControl)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(transparent1x1PNG)
}

func (p *ImageProxy) cloudImageCachePaths(stableKey string) (string, string, string) {
	stableKey = strings.TrimSpace(stableKey)
	if stableKey == "" {
		stableKey = "unknown"
	}
	sum := sha256.Sum256([]byte("cloud-image:" + stableKey))
	key := "cloud-" + hex.EncodeToString(sum[:])
	cachePath := filepath.Join(p.cacheDir, key)
	return key, cachePath, cachePath + ".fail"
}

func (p *ImageProxy) remoteImageCachePaths(raw string) (string, string, string, error) {
	if _, err := p.validateURL(raw); err != nil {
		return "", "", "", err
	}
	key, cachePath, failPath := p.remoteImageCachePathsForValidated(raw)
	return key, cachePath, failPath, nil
}

func (p *ImageProxy) remoteImageCachePathsForValidated(raw string) (string, string, string) {
	sum := sha256.Sum256([]byte(raw))
	key := hex.EncodeToString(sum[:])
	cachePath := filepath.Join(p.cacheDir, key)
	return key, cachePath, cachePath + ".fail"
}

const (
	defaultImageCacheTTLHours      = 30 * 24
	defaultImageCacheMaxMB         = 512
	defaultImageCachePruneInterval = 15 * time.Minute
)

func (p *ImageProxy) imageCacheTTL() time.Duration {
	ttlHours := defaultImageCacheTTLHours
	if p != nil && p.cfg != nil && p.cfg.Cache.ImageCacheTTLHours > 0 {
		ttlHours = p.cfg.Cache.ImageCacheTTLHours
	}
	return time.Duration(ttlHours) * time.Hour
}

func (p *ImageProxy) imageCacheLimitBytes() int64 {
	maxMB := defaultImageCacheMaxMB
	if p != nil && p.cfg != nil && p.cfg.Cache.ImageCacheMaxMB > 0 {
		maxMB = p.cfg.Cache.ImageCacheMaxMB
	}
	maxBytes := int64(maxMB) << 20
	if maxBytes < 1 {
		return int64(defaultImageCacheMaxMB) << 20
	}
	return maxBytes
}

func (p *ImageProxy) imageCachePruneInterval() time.Duration {
	intervalMinutes := int(defaultImageCachePruneInterval / time.Minute)
	if p != nil && p.cfg != nil && p.cfg.Cache.ImageCachePruneIntervalMin > 0 {
		intervalMinutes = p.cfg.Cache.ImageCachePruneIntervalMin
	}
	return time.Duration(intervalMinutes) * time.Minute
}

func (p *ImageProxy) initializeImageCache() {
	if p == nil {
		return
	}
	p.mu.Lock()
	total, err := p.pruneImageCache()
	p.imageCacheMu.Lock()
	p.imageCacheBytes = total
	p.imageCacheLastPruned = time.Now()
	p.imageCacheMu.Unlock()
	p.mu.Unlock()
	if p.log != nil {
		if err != nil {
			p.log.Warn("imageproxy: original cache startup prune failed", zap.String("dir", p.cacheDir), zap.Error(err))
		} else {
			p.log.Info("imageproxy: original cache policy initialized",
				zap.String("dir", p.cacheDir),
				zap.Int("ttl_hours", int(p.imageCacheTTL()/time.Hour)),
				zap.Int64("max_bytes", p.imageCacheLimitBytes()),
				zap.Int64("bytes", total),
			)
		}
	}
}

func (p *ImageProxy) scheduleImageCachePrune(force bool) {
	if p == nil {
		return
	}
	p.imageCacheMu.Lock()
	if p.imageCachePruning || (!force && !p.imageCacheLastPruned.IsZero() && time.Since(p.imageCacheLastPruned) < p.imageCachePruneInterval()) {
		p.imageCacheMu.Unlock()
		return
	}
	p.imageCachePruning = true
	p.imageCacheMu.Unlock()

	go func() {
		p.mu.Lock()
		total, err := p.pruneImageCache()
		p.imageCacheMu.Lock()
		p.imageCacheBytes = total
		p.imageCacheLastPruned = time.Now()
		p.imageCachePruning = false
		p.imageCacheMu.Unlock()
		p.mu.Unlock()
		if err != nil && p.log != nil {
			p.log.Warn("imageproxy: original cache prune failed", zap.String("dir", p.cacheDir), zap.Error(err))
		}
	}()
}

func (p *ImageProxy) noteImageCacheWrite(previousSize, newSize int64) {
	if p == nil {
		return
	}
	p.imageCacheMu.Lock()
	p.imageCacheBytes += newSize - previousSize
	if p.imageCacheBytes < 0 {
		p.imageCacheBytes = 0
	}
	force := p.imageCacheBytes > p.imageCacheLimitBytes()
	p.imageCacheMu.Unlock()
	p.scheduleImageCachePrune(force)
}

func (p *ImageProxy) pruneImageCache() (int64, error) {
	return pruneImageCacheDir(p.cacheDir, time.Now().Add(-p.imageCacheTTL()), p.imageCacheLimitBytes())
}

type imageCacheEntry struct {
	path    string
	size    int64
	modTime time.Time
}

// pruneImageCacheDir only considers positive remote/cloud image cache entries
// in root. Variants, short-lived .fail entries, and in-progress temp files use
// their own lifecycles and are deliberately left untouched.
func pruneImageCacheDir(root string, cutoff time.Time, maxBytes int64) (int64, error) {
	if strings.TrimSpace(root) == "" {
		return 0, nil
	}
	dirEntries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if maxBytes < 0 {
		maxBytes = 0
	}

	var entries []imageCacheEntry
	var total int64
	var firstErr error
	setErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, dirEntry := range dirEntries {
		if !isOriginalImageCacheFile(dirEntry.Name()) {
			continue
		}
		info, err := dirEntry.Info()
		if err != nil {
			setErr(err)
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		path := filepath.Join(root, dirEntry.Name())
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err == nil || os.IsNotExist(err) {
				continue
			} else {
				setErr(err)
			}
		}
		total += info.Size()
		entries = append(entries, imageCacheEntry{path: path, size: info.Size(), modTime: info.ModTime()})
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].modTime.Equal(entries[j].modTime) {
			return entries[i].path < entries[j].path
		}
		return entries[i].modTime.Before(entries[j].modTime)
	})
	for _, entry := range entries {
		if total <= maxBytes {
			break
		}
		if err := os.Remove(entry.path); err != nil {
			setErr(err)
			continue
		}
		total -= entry.size
	}
	return total, firstErr
}

func isOriginalImageCacheFile(name string) bool {
	key := strings.TrimPrefix(strings.TrimSpace(name), "cloud-")
	if len(key) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(key)
	return err == nil
}

const (
	imageVariantCacheTTL                 = 30 * 24 * time.Hour
	imageVariantCacheMaxBytes      int64 = 256 << 20
	imageVariantCacheFileMaxBytes        = 2 << 20
	imageVariantCachePruneInterval       = 15 * time.Minute
)

func (p *ImageProxy) imageVariantCacheDir() string {
	return filepath.Join(p.cacheDir, "variants")
}

func imageVariantCacheDigest(values ...string) string {
	h := sha256.New()
	for _, value := range values {
		_, _ = h.Write([]byte(value))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func imageVariantSourceCacheKey(sourceKey string) string {
	return imageVariantCacheDigest("image-variant-source:v1", strings.TrimSpace(sourceKey))
}

// imageVariantCachePaths derives a cache entry from the stable source key,
// its on-disk version, and the effective output options. The source and
// version directories let one source be purged without touching unrelated
// cached variants. Including source size and mtime makes a changed local image
// or refreshed remote cache use a new derived image automatically.
func (p *ImageProxy) imageVariantCachePaths(sourceKey string, sourceModTime time.Time, sourceSize int64, variant imageVariantOptions) (string, string) {
	quality := variant.quality
	if quality <= 0 {
		quality = defaultImageVariantQuality
	}
	quality = clampImageQuality(quality)

	values := []string{
		strings.TrimSpace(sourceKey),
		strconv.FormatInt(sourceSize, 10),
		strconv.FormatInt(sourceModTime.UnixNano(), 10),
		strconv.Itoa(variant.maxWidth),
		strconv.Itoa(variant.maxHeight),
		strconv.Itoa(quality),
		strconv.FormatBool(variant.hasQuality),
	}
	key := "variant-" + imageVariantCacheDigest(append([]string{"image-variant:v1"}, values...)...)
	versionKey := imageVariantCacheDigest("image-variant-version:v1", values[0], values[1], values[2])
	return key, filepath.Join(p.imageVariantCacheDir(), imageVariantSourceCacheKey(sourceKey), versionKey, key)
}

func (p *ImageProxy) removeImageVariantCache(sourceKey string) error {
	if p == nil {
		return nil
	}
	dir := filepath.Join(p.imageVariantCacheDir(), imageVariantSourceCacheKey(sourceKey))
	p.variantCacheMu.Lock()
	defer p.variantCacheMu.Unlock()
	size, err := imageVariantCacheDirSize(dir)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	if size >= p.variantCacheBytes {
		p.variantCacheBytes = 0
	} else {
		p.variantCacheBytes -= size
	}
	return nil
}

func (p *ImageProxy) initializeImageVariantCache() {
	if p == nil {
		return
	}
	p.variantCacheMu.Lock()
	total, err := p.pruneImageVariantCache(imageVariantCacheMaxBytes)
	if err != nil {
		total = imageVariantCacheMaxBytes
	}
	p.variantCacheBytes = total
	p.variantCacheLastPruned = time.Now()
	p.variantCacheMu.Unlock()
	if err != nil && p.log != nil {
		p.log.Warn("imageproxy: variant cache startup prune failed", zap.String("dir", p.imageVariantCacheDir()), zap.Error(err))
	}
}

func (p *ImageProxy) scheduleImageVariantCachePrune(force bool) {
	if p == nil {
		return
	}
	p.variantCacheMu.Lock()
	if p.variantCachePruning || (!force && !p.variantCacheLastPruned.IsZero() && time.Since(p.variantCacheLastPruned) < imageVariantCachePruneInterval) {
		p.variantCacheMu.Unlock()
		return
	}
	p.variantCachePruning = true
	p.variantCacheMu.Unlock()

	maxBytes := imageVariantCacheMaxBytes
	if force {
		maxBytes -= imageVariantCacheFileMaxBytes
	}
	go func() {
		p.variantCacheMu.Lock()
		total, err := p.pruneImageVariantCache(maxBytes)
		if err == nil {
			p.variantCacheBytes = total
		}
		p.variantCacheLastPruned = time.Now()
		p.variantCachePruning = false
		p.variantCacheMu.Unlock()
		if err != nil && p.log != nil {
			p.log.Warn("imageproxy: variant cache prune failed", zap.String("dir", p.imageVariantCacheDir()), zap.Error(err))
		}
	}()
}

func (p *ImageProxy) pruneImageVariantCache(maxBytes int64) (int64, error) {
	return pruneImageVariantCacheDir(p.imageVariantCacheDir(), time.Now().Add(-imageVariantCacheTTL), maxBytes)
}

type imageVariantCacheEntry struct {
	path    string
	size    int64
	modTime time.Time
}

func pruneImageVariantCacheDir(root string, cutoff time.Time, maxBytes int64) (int64, error) {
	if strings.TrimSpace(root) == "" {
		return 0, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if maxBytes < 0 {
		maxBytes = 0
	}

	var entries []imageVariantCacheEntry
	var dirs []string
	var total int64
	var firstErr error
	setErr := func(err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			setErr(walkErr)
			return nil
		}
		if info == nil {
			return nil
		}
		if info.IsDir() {
			if path != root {
				dirs = append(dirs, path)
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err == nil || os.IsNotExist(err) {
				return nil
			} else {
				setErr(err)
			}
		}
		total += info.Size()
		entries = append(entries, imageVariantCacheEntry{path: path, size: info.Size(), modTime: info.ModTime()})
		return nil
	})
	setErr(err)

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].modTime.Equal(entries[j].modTime) {
			return entries[i].path < entries[j].path
		}
		return entries[i].modTime.Before(entries[j].modTime)
	})
	for _, entry := range entries {
		if total <= maxBytes {
			break
		}
		if err := os.Remove(entry.path); err != nil {
			setErr(err)
			continue
		}
		total -= entry.size
	}
	for i := len(dirs) - 1; i >= 0; i-- {
		children, err := os.ReadDir(dirs[i])
		if err != nil {
			if !os.IsNotExist(err) {
				setErr(err)
			}
			continue
		}
		if len(children) != 0 {
			continue
		}
		if err := os.Remove(dirs[i]); err != nil && !os.IsNotExist(err) {
			setErr(err)
		}
	}
	return total, firstErr
}

func imageVariantCacheDirSize(root string) (int64, error) {
	var total int64
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	err := filepath.Walk(root, func(_ string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info != nil && info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func (p *ImageProxy) serveCachedImageFile(w http.ResponseWriter, r *http.Request, key, cachePath string) bool {
	return p.serveImageFile(w, r, key, cachePath, imageBrowserCacheControl)
}

func (p *ImageProxy) serveCachedImageVariantFile(w http.ResponseWriter, r *http.Request, key, cachePath, cacheControl string) bool {
	return p.serveImageFileWithVariant(w, r, key, cachePath, cacheControl, false, imageVariantCacheETag(key))
}

func (p *ImageProxy) serveImageFile(w http.ResponseWriter, r *http.Request, key, path, cacheControl string) bool {
	return p.serveImageFileWithVariant(w, r, key, path, cacheControl, true, "")
}

func (p *ImageProxy) serveImageFileWithVariant(w http.ResponseWriter, r *http.Request, key, path, cacheControl string, allowVariant bool, etag string) bool {
	file, err := os.Open(path) // #nosec G304 -- caller only passes validated local paths or SHA-derived cache paths.
	if err != nil {
		return false
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil || stat.IsDir() || stat.Size() <= 0 {
		return false
	}
	var sample [512]byte
	n, _ := file.Read(sample[:])
	_, _ = file.Seek(0, io.SeekStart)
	ctype := detectContentType(sample[:n])
	if !isImageContentType(ctype) {
		return false
	}
	if variant := imageVariantFromRequest(r); allowVariant && variant.enabled() {
		if stat.Size() > maxImageVariantInputBytes {
			p.serveImageVariantFailure(w, key, errors.New("image exceeds variant input limit"))
			return true
		}
		data, err := io.ReadAll(io.LimitReader(file, maxImageVariantInputBytes))
		if err != nil || len(data) == 0 {
			if err == nil {
				err = errors.New("image variant source is empty")
			}
			p.serveImageVariantFailure(w, key, err)
			return true
		}
		if err := p.serveImageVariant(w, r, key, stat.ModTime(), stat.Size(), data, ctype, cacheControl, variant); err != nil {
			p.serveImageVariantFailure(w, key, err)
		}
		return true
	}
	if etag == "" {
		etag = imageFileETag(key, stat)
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", etag)
	http.ServeContent(w, r, key, stat.ModTime(), file)
	return true
}

func (p *ImageProxy) serveImageBytes(w http.ResponseWriter, r *http.Request, key string, modTime time.Time, data []byte, contentType, contentLength, cacheControl string) {
	if variant := imageVariantFromRequest(r); variant.enabled() {
		if len(data) > maxImageVariantInputBytes {
			p.serveImageVariantFailure(w, key, errors.New("image exceeds variant input limit"))
			return
		}
		if err := p.serveImageVariant(w, r, key, modTime, int64(len(data)), data, contentType, cacheControl, variant); err != nil {
			p.serveImageVariantFailure(w, key, err)
		}
		return
	}
	w.Header().Set("Content-Type", contentType)
	if contentLength != "" {
		w.Header().Set("Content-Length", contentLength)
	}
	w.Header().Set("Cache-Control", cacheControl)
	http.ServeContent(w, r, key, modTime, bytes.NewReader(data))
}

func (p *ImageProxy) serveImageVariantFailure(w http.ResponseWriter, key string, err error) {
	if p != nil && p.log != nil {
		p.log.Warn("imageproxy: image variant generation failed", zap.String("key", key), zap.Error(err))
	}
	w.Header().Del("Content-Type")
	w.Header().Del("Content-Length")
	w.Header().Del("ETag")
	w.Header().Del("Last-Modified")
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, "image variant generation failed", http.StatusUnprocessableEntity)
}

func imageFileETag(key string, stat os.FileInfo) string {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "image"
	}
	sum := sha256.Sum256([]byte(key))
	return `"img-` + hex.EncodeToString(sum[:8]) + "-" + strconv.FormatInt(stat.Size(), 16) + "-" + strconv.FormatInt(stat.ModTime().Unix(), 16) + `"`
}

func imageVariantCacheETag(key string) string {
	sum := sha256.Sum256([]byte("image-variant:" + key))
	return `"imgv-` + hex.EncodeToString(sum[:12]) + `"`
}

func freshNegativeImageCache(failPath string) bool {
	stat, err := os.Stat(failPath)
	if err != nil {
		return false
	}
	if time.Since(stat.ModTime()) < imageNegativeCacheTTL {
		return true
	}
	_ = os.Remove(failPath)
	return false
}

func (p *ImageProxy) markImageFetchFailed(failPath string) {
	if err := os.MkdirAll(filepath.Dir(failPath), 0o750); err != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = os.WriteFile(failPath, []byte(time.Now().Format(time.RFC3339Nano)), 0o600)
}
