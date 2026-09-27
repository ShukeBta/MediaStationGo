package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type asrAudioSource struct {
	input   string
	headers map[string]string
}

type ASRAudioInfo struct {
	DurationSeconds int
	BitrateBPS      int
}

// StartASRAudioExtraction starts a single ffmpeg process that converts the
// first audio stream of a media version into a compact mono MP3 stream.
func (s *StreamService) StartASRAudioExtraction(ctx context.Context, mediaID string) (io.ReadCloser, func() error, ASRAudioInfo, error) {
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return nil, nil, ASRAudioInfo{}, errors.New("media stream service unavailable")
	}
	media, err := s.repo.Media.FindByID(ctx, strings.TrimSpace(mediaID))
	if err != nil {
		return nil, nil, ASRAudioInfo{}, err
	}
	if media == nil {
		return nil, nil, ASRAudioInfo{}, ErrMediaNotFound
	}
	source, err := s.resolveASRAudioSource(ctx, media)
	if err != nil {
		return nil, nil, ASRAudioInfo{}, err
	}
	bin, err := resolveLocalExecutable(s.cfg.App.FFmpegPath, "ffmpeg")
	if err != nil {
		return nil, nil, ASRAudioInfo{}, fmt.Errorf("ffmpeg unavailable: %w", err)
	}
	s.cfg.App.FFmpegPath = bin
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
	if headerText := ffmpegHeaderText(source.headers); headerText != "" {
		args = append(args, "-headers", headerText)
	}
	args = append(args,
		"-i", source.input,
		"-map", "0:a:0",
		"-vn", "-sn", "-dn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "libmp3lame",
		"-b:a", "48k",
		"-f", "mp3",
		"pipe:1",
	)
	cmd := exec.CommandContext(ctx, bin, args...) // #nosec G204 -- executable is resolved locally and arguments are not shell-expanded.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, ASRAudioInfo{}, fmt.Errorf("open ffmpeg audio output: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		return nil, nil, ASRAudioInfo{}, fmt.Errorf("start ffmpeg audio extraction: %w", err)
	}
	wait := func() error {
		if err := cmd.Wait(); err != nil {
			detail := strings.TrimSpace(stderr.String())
			return errors.New(ffmpegASRAudioFailure(detail))
		}
		return nil
	}
	return stdout, wait, ASRAudioInfo{DurationSeconds: media.DurationSec, BitrateBPS: 48000}, nil
}

func ffmpegASRAudioFailure(detail string) string {
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "matches no streams") || strings.Contains(lower, "does not contain any stream"):
		return "ffmpeg audio extraction failed: media has no audio stream"
	case strings.Contains(lower, "403 forbidden") || strings.Contains(lower, "server returned 403"):
		return "ffmpeg audio extraction failed: upstream returned HTTP 403"
	case strings.Contains(lower, "401 unauthorized") || strings.Contains(lower, "server returned 401"):
		return "ffmpeg audio extraction failed: upstream returned HTTP 401"
	case strings.Contains(lower, "404 not found") || strings.Contains(lower, "server returned 404"):
		return "ffmpeg audio extraction failed: upstream media was not found"
	case strings.Contains(lower, "invalid data found"):
		return "ffmpeg audio extraction failed: media data is invalid or unsupported"
	default:
		return "ffmpeg audio extraction failed"
	}
}

func (s *StreamService) resolveASRAudioSource(ctx context.Context, media *model.Media) (asrAudioSource, error) {
	if media == nil {
		return asrAudioSource{}, ErrMediaNotFound
	}
	if typ, ref, ok := parseCloudMediaPlaybackURL(media.STRMURL); ok {
		if s.storage == nil {
			return asrAudioSource{}, errors.New("cloud media audio extraction unavailable: storage service not configured")
		}
		link, err := s.storage.CloudResolve(ctx, typ, ref, cloudMediaInternalUserAgent)
		if err != nil {
			return asrAudioSource{}, err
		}
		if link == nil || strings.TrimSpace(link.URL) == "" {
			return asrAudioSource{}, errors.New("cloud media resolved to an empty URL")
		}
		return asrAudioSource{input: link.URL, headers: cloudMediaInternalHeaders(link.Headers)}, nil
	}
	if rawURL := probeHTTPMediaURL(media); rawURL != "" {
		return asrAudioSource{input: rawURL, headers: cloudMediaInternalHeaders(nil)}, nil
	}
	path := strings.TrimSpace(media.Path)
	if path == "" {
		return asrAudioSource{}, errors.New("media audio extraction unavailable: empty media path")
	}
	if strings.HasPrefix(strings.ToLower(path), "cloud://") {
		return asrAudioSource{}, errors.New("cloud media audio extraction unavailable: missing resolvable playback reference; re-scan the library")
	}
	return asrAudioSource{input: path}, nil
}
