// 本文件提供本地视频缩略图回退：当媒体没有刮削封面时，用 ffmpeg 截帧生成
// Primary 图。函数从旧版 emby_compat.go 拆出（pr/local-video-thumbnail-fallback），
// 配合 emby_artwork.go 与 emby_series_payload.go 的接线使用。
package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var embyLocalThumbnailSem = make(chan struct{}, 2)

func (e *EmbyService) firstLocalThumbnailMedia(rows []model.Media) *model.Media {
	for i := range rows {
		if e.mediaCanAdvertiseLocalThumbnail(&rows[i]) {
			return &rows[i]
		}
	}
	return nil
}

func (e *EmbyService) mediaRowsCanGenerateLocalThumbnail(rows []model.Media) bool {
	return e.firstLocalThumbnailMedia(rows) != nil
}

func (e *EmbyService) localThumbnailFromMediaRows(ctx context.Context, rows []model.Media) (string, error) {
	m := e.firstLocalThumbnailMedia(rows)
	if m == nil {
		return "", nil
	}
	return e.localVideoThumbnail(ctx, m)
}

func embyWantsPrimaryImage(imageType string) bool {
	imageType = strings.ToLower(strings.TrimSpace(imageType))
	return imageType == "" || imageType == "primary"
}

func (e *EmbyService) mediaCanGenerateLocalThumbnail(m *model.Media) bool {
	if e == nil || e.cfg == nil || m == nil {
		return false
	}
	if strings.TrimSpace(e.cfg.Cache.CacheDir) == "" {
		return false
	}
	if strings.TrimSpace(m.PosterURL) != "" || strings.TrimSpace(m.STRMURL) != "" {
		return false
	}
	path := strings.TrimSpace(m.Path)
	if path == "" || isHTTPish(path) || strings.HasPrefix(strings.ToLower(path), "cloud://") {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".mkv", ".avi", ".mov", ".m4v", ".webm", ".ts", ".m2ts", ".wmv", ".flv", ".rmvb":
		return true
	default:
		return false
	}
}

func (e *EmbyService) mediaCanAdvertiseLocalThumbnail(m *model.Media) bool {
	if !e.mediaCanGenerateLocalThumbnail(m) {
		return false
	}
	source, err := filepath.Abs(filepath.Clean(m.Path))
	if err != nil {
		return false
	}
	// 每条媒体两次 os.Stat 在慢速存储上会拖垮大列表，按路径缓存两分钟。
	cacheKey := "thumbadv|" + source
	if cached, ok := e.cachedFSProbe(cacheKey); ok {
		return cached == "1"
	}
	ok := e.probeLocalThumbnailAdvertise(source)
	if ok {
		e.storeFSProbe(cacheKey, "1")
	} else {
		e.storeFSProbe(cacheKey, "0")
	}
	return ok
}

func (e *EmbyService) probeLocalThumbnailAdvertise(source string) bool {
	if stat, err := os.Stat(source); err != nil || stat.IsDir() {
		return false
	}
	cachePath, failPath := e.localVideoThumbnailPaths(source)
	if stat, err := os.Stat(cachePath); err == nil && stat.Size() > 0 {
		return true
	}
	if freshNegativeImageCache(failPath) {
		data, _ := os.ReadFile(failPath) // #nosec G304 -- failPath is derived from a SHA-256 cache key under the configured cache directory.
		if !localVideoThumbnailFailureRetryable(string(data)) {
			return false
		}
		_ = os.Remove(failPath)
	}
	return true
}

func (e *EmbyService) localVideoThumbnail(ctx context.Context, m *model.Media) (string, error) {
	if !e.mediaCanGenerateLocalThumbnail(m) {
		return "", nil
	}
	source, err := filepath.Abs(filepath.Clean(m.Path))
	if err != nil {
		return "", nil
	}
	if stat, err := os.Stat(source); err != nil || stat.IsDir() {
		return "", nil
	}
	cachePath, failPath := e.localVideoThumbnailPaths(source)
	if stat, err := os.Stat(cachePath); err == nil && stat.Size() > 0 {
		return cachePath, nil
	}
	if freshNegativeImageCache(failPath) {
		data, _ := os.ReadFile(failPath) // #nosec G304 -- failPath is derived from a SHA-256 cache key under the configured cache directory.
		if !localVideoThumbnailFailureRetryable(string(data)) {
			return "", nil
		}
		_ = os.Remove(failPath)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o750); err != nil {
		return "", err
	}
	bin, err := resolveLocalExecutable(e.cfg.App.FFmpegPath, "ffmpeg")
	if err != nil {
		_ = os.WriteFile(failPath, []byte(err.Error()), 0o600)
		return "", nil
	}
	select {
	case embyLocalThumbnailSem <- struct{}{}:
		defer func() { <-embyLocalThumbnailSem }()
	case <-ctx.Done():
		return "", nil
	}
	thumbCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	tmp := cachePath + ".tmp.jpg"
	_ = os.Remove(tmp)
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-nostdin",
		"-ss", localThumbnailSeekPosition(m.DurationSec),
		"-noaccurate_seek",
		"-i", source,
		"-map", "0:v:0",
		"-frames:v", "1",
		"-vf", `scale=min(480\,iw):-2`,
		"-q:v", "6",
		"-y", tmp,
	}
	output, err := exec.CommandContext(thumbCtx, bin, args...).CombinedOutput() // #nosec G204 -- ffmpeg path is resolved locally and args are not shell-expanded.
	if err != nil {
		_ = os.Remove(tmp)
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		} else {
			message = err.Error() + ": " + message
		}
		if localVideoThumbnailFailureRetryable(message) || thumbCtx.Err() != nil {
			_ = os.Remove(failPath)
			return "", nil
		}
		_ = os.WriteFile(failPath, []byte(message), 0o600)
		return "", nil
	}
	if stat, err := os.Stat(tmp); err != nil || stat.Size() == 0 {
		_ = os.Remove(tmp)
		_ = os.WriteFile(failPath, []byte("empty thumbnail"), 0o600)
		return "", nil
	}
	if err := os.Rename(tmp, cachePath); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	_ = os.Remove(failPath)
	return cachePath, nil
}

func (e *EmbyService) localVideoThumbnailPaths(source string) (string, string) {
	cacheDir := filepath.Join(e.cfg.Cache.CacheDir, "images", "video-thumbs")
	sum := sha256.Sum256([]byte(source))
	cachePath := filepath.Join(cacheDir, hex.EncodeToString(sum[:])+".jpg")
	return cachePath, cachePath + ".fail"
}

func localThumbnailSeekPosition(durationSec int) string {
	switch {
	case durationSec <= 2:
		return "00:00:00"
	case durationSec < 10:
		return "00:00:01"
	default:
		return "00:00:02"
	}
}

func localVideoThumbnailFailureRetryable(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	if message == "" {
		return false
	}
	for _, needle := range []string{
		"exit status 234",
		"signal: killed",
		"context canceled",
		"context deadline exceeded",
		"deadline exceeded",
		"operation was canceled",
	} {
		if strings.Contains(message, needle) {
			return true
		}
	}
	return false
}
