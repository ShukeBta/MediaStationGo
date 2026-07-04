package service

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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
	f, err := os.Open(playlist) // #nosec G304 -- playlist path is generated under the transcoder cache directory for this media ID.
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
	if !pathWithin(abs, dir) {
		return errors.New("path escape")
	}
	f, err := os.Open(abs) // #nosec G304 -- abs is constrained to the HLS cache directory with pathWithin.
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
