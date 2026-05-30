// Package service — direct-play / HLS streaming.
//
// StreamService exposes two flavours of playback:
//
//   - Direct play: the original file is served with HTTP Range support.
//     Works for browser-friendly containers (mp4 / webm / m4v), no ffmpeg
//     involved, zero CPU overhead.
//   - HLS: when the client opts in (or the source codec / container is
//     not browser-friendly), the TranscoderService runs ffmpeg in the
//     background and we serve the resulting .m3u8 + .ts files directly.
//
// The HTTP layer decides which mode to use based on the request path:
//
//	GET /api/stream/:id              → direct play
//	GET /api/hls/:id/index.m3u8      → HLS playlist
//	GET /api/hls/:id/seg_NNNNN.ts    → HLS segment
package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// StreamService serves media files with proper Range support so browsers can
// seek into the stream.
type StreamService struct {
	cfg        *config.Config
	log        *zap.Logger
	repo       *repository.Container
	transcoder *TranscoderService
}

// NewStreamService is the constructor.
func NewStreamService(cfg *config.Config, log *zap.Logger, repo *repository.Container, transcoder *TranscoderService) *StreamService {
	return &StreamService{
		cfg:        cfg,
		log:        log,
		repo:       repo,
		transcoder: transcoder,
	}
}

// ErrMediaNotFound is returned when the media row or its file is missing.
var ErrMediaNotFound = errors.New("media not found")

// directPlayOnly reports whether the admin enabled「客户端直连解码」mode,
// in which the host never transcodes (HLS is refused) and all playback is
// handled by the client (direct play / 302 redirect).
func (s *StreamService) directPlayOnly(ctx context.Context) bool {
	if s.repo == nil || s.repo.Setting == nil {
		return false
	}
	v, err := s.repo.Setting.Get(ctx, PlaybackDirectOnlySettingKey)
	if err != nil {
		return false
	}
	return parseBoolSetting(v, false)
}

// ServeFile streams the file backing the given media ID using
// http.ServeContent so HEAD / Range / If-Modified-Since are handled for free.
//
// When the media row has a STRMURL set we redirect (302) to that URL
// instead of opening a local file. This lets WebDAV / Alist / S3 / HTTP
// direct links flow through the rest of the player UI unchanged.
func (s *StreamService) ServeFile(w http.ResponseWriter, r *http.Request, mediaID string) error {
	m, err := s.repo.Media.FindByID(r.Context(), mediaID)
	if err != nil {
		return err
	}
	if m == nil {
		return ErrMediaNotFound
	}
	if strings.TrimSpace(m.STRMURL) != "" {
		http.Redirect(w, r, m.STRMURL, http.StatusFound)
		return nil
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

// ServeHLSPlaylist makes sure a transcode is running and writes the m3u8.
// We block (with a 30s timeout) until the playlist file shows up.
func (s *StreamService) ServeHLSPlaylist(w http.ResponseWriter, r *http.Request, mediaID string) error {
	// 「客户端直连解码」模式下宿主机不提供转码，HLS 一律拒绝，
	// 迫使播放器走 direct play 本地解码。
	if s.directPlayOnly(r.Context()) {
		return ErrTranscodeDisabled
	}
	if _, err := s.transcoder.EnsureJob(r.Context(), mediaID); err != nil {
		return err
	}
	s.transcoder.TouchJob(mediaID)
	if !s.transcoder.WaitReady(r.Context(), mediaID, 30*time.Second) {
		return errors.New("hls playlist not ready")
	}
	playlist := s.transcoder.PlaylistPath(mediaID)
	f, err := os.Open(playlist)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, _ := f.Stat()
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Disposition", "inline")
	if r.URL.RawQuery != "" {
		data, err := io.ReadAll(f)
		if err != nil {
			return err
		}
		playlist := appendQueryToHLSSegments(string(data), r.URL.RawQuery)
		_, err = io.WriteString(w, playlist)
		return err
	}
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	return nil
}

func appendQueryToHLSSegments(playlist, rawQuery string) string {
	if strings.TrimSpace(rawQuery) == "" {
		return playlist
	}
	lines := strings.SplitAfter(playlist, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.Contains(trimmed, "?") {
			continue
		}
		if strings.HasSuffix(strings.ToLower(trimmed), ".ts") {
			lineEnding := ""
			if strings.HasSuffix(line, "\r\n") {
				lineEnding = "\r\n"
			} else if strings.HasSuffix(line, "\n") {
				lineEnding = "\n"
			}
			lines[i] = strings.TrimRight(line, "\r\n") + "?" + rawQuery + lineEnding
		}
	}
	return strings.Join(lines, "")
}

// ServeHLSSegment writes a single .ts segment from the on-disk cache.
func (s *StreamService) ServeHLSSegment(w http.ResponseWriter, r *http.Request, mediaID, segment string) error {
	s.transcoder.TouchJob(mediaID)
	// Only allow segments that look like seg_NNNNN.ts so we cannot be tricked
	// into reading arbitrary files via path traversal.
	if !strings.HasPrefix(segment, "seg_") || !strings.HasSuffix(segment, ".ts") {
		return errors.New("bad segment")
	}
	full := filepath.Join(s.transcoder.HLSDir(mediaID), segment)
	abs, err := filepath.Abs(full)
	if err != nil {
		return err
	}
	dir, _ := filepath.Abs(s.transcoder.HLSDir(mediaID))
	if !strings.HasPrefix(abs, dir) {
		return errors.New("path escape")
	}
	f, err := os.Open(abs)
	if err != nil {
		return err
	}
	defer f.Close()
	stat, _ := f.Stat()
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, stat.Name(), stat.ModTime(), f)
	return nil
}

// Probe re-runs ffprobe against an existing media row and refreshes the
// extracted metadata. Used by the admin UI's "rescan" button.
func (s *StreamService) Probe(ctx context.Context, mediaID string, probe *FFprobeService) error {
	m, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return ErrMediaNotFound
	}
	res, err := probe.Probe(ctx, m.Path)
	if err != nil {
		return err
	}
	updates := map[string]any{
		"duration_sec": res.DurationSec,
		"width":        res.Width,
		"height":       res.Height,
		"video_codec":  res.VideoCodec,
		"audio_codec":  res.AudioCodec,
	}
	if res.Container != "" {
		updates["container"] = res.Container
	}
	return s.repo.DB.Model(m).Updates(updates).Error
}
