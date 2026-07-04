package service

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// ServeFile streams the file backing the given media ID using
// http.ServeContent so HEAD / Range / If-Modified-Since are handled for free.
//
// When the media row has a STRMURL set we redirect (302) to that URL
// instead of opening a local file. This lets WebDAV / Alist / S3 / HTTP
// direct links flow through the rest of the player UI unchanged.
func (s *StreamService) ServeFile(w http.ResponseWriter, r *http.Request, mediaID string) error {
	return s.ServeFileWithCloudMode(w, r, mediaID, "")
}

func (s *StreamService) ServeFileWithCloudMode(w http.ResponseWriter, r *http.Request, mediaID, cloudMode string) error {
	m, err := s.repo.Media.FindByID(r.Context(), mediaID)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrMediaNotFound
	}
	if strmURL := strings.TrimSpace(m.STRMURL); strmURL != "" && playableSTRMTarget(r.Context(), s.repo, strmURL, m) {
		if !cloudPlaybackModeEnabled(r.Context(), s.repo, cloudMode) {
			return ErrCloudPlaybackDisabled
		}
		// 云盘播放 URL 先规范化为相对路径，免疫扫描时固化的旧 host。
		target := normalizeCloudPlayTarget(strmURL)
		target = withAuthTokenForInternalRedirect(target, r, PublicServerURL(r.Context(), s.repo, s.cfg))
		setCloudRedirectNoStore(w)
		http.Redirect(w, r, absoluteInternalRedirect(target, r), http.StatusFound)
		return nil
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(m.Path)), "cloud://") {
		// 云盘媒体没有本地文件可回退；走到这里说明 STRM 播放被关闭或
		// STRMURL 缺失。返回明确错误而不是笼统的「文件不存在」，
		// 处理器据此回 502 + 原因，方便用户在播放器/日志里定位。
		return ErrCloudPlaybackUnavailable
	}
	f, err := os.Open(m.Path)
	if err != nil {
		return ErrMediaNotFound
	}
	defer f.Close()
	stat, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	return nil
}

func setCloudRedirectNoStore(w http.ResponseWriter) {
	if w == nil {
		return
	}
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
}

func isCloudPlaybackTarget(raw string) bool {
	_, _, ok := parseCloudMediaPlaybackURL(raw)
	return ok
}

func playableSTRMTarget(ctx context.Context, repo *repository.Container, raw string, m *model.Media) bool {
	if isCloudPlaybackTarget(raw) || isHTTPPlaybackTarget(raw) {
		return true
	}
	if m != nil && strings.EqualFold(strings.TrimSpace(m.Container), "strm") {
		return true
	}
	return STRMPlaybackEnabled(ctx, repo)
}

func isHTTPPlaybackTarget(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || !u.IsAbs() {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	return scheme == "http" || scheme == "https"
}
