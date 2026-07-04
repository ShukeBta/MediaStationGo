package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

func serveCachedImageFile(w http.ResponseWriter, r *http.Request, key, cachePath string) bool {
	return serveImageFile(w, r, key, cachePath, imageBrowserCacheControl)
}

func serveImageFile(w http.ResponseWriter, r *http.Request, key, path, cacheControl string) bool {
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
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", cacheControl)
	w.Header().Set("ETag", imageFileETag(key, stat))
	http.ServeContent(w, r, key, stat.ModTime(), file)
	return true
}

func imageFileETag(key string, stat os.FileInfo) string {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "image"
	}
	sum := sha256.Sum256([]byte(key))
	return `"img-` + hex.EncodeToString(sum[:8]) + "-" + strconv.FormatInt(stat.Size(), 16) + "-" + strconv.FormatInt(stat.ModTime().Unix(), 16) + `"`
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
