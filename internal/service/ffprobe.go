// Package service — ffprobe wrapper.
//
// FFprobeService shells out to the `ffprobe` binary configured in
// app.ffprobe_path and parses its JSON output into a typed struct. It is
// intentionally minimal: we only extract the fields needed to populate
// model.Media (duration, resolution, video / audio codec) so a fresh scan
// can show meaningful metadata even before the TMDb scraper has run.
package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// FFprobeService wraps the external ffprobe binary.
type FFprobeService struct {
	cfg     *config.Config
	log     *zap.Logger
	mu      sync.RWMutex
	limiter chan struct{}
}

// NewFFprobeService is the constructor.
func NewFFprobeService(cfg *config.Config, log *zap.Logger) *FFprobeService {
	maxConcurrent := normalizeFFprobeMaxConcurrent(cfg.App.FFprobeMaxConcurrent)
	return &FFprobeService{cfg: cfg, log: log, limiter: make(chan struct{}, maxConcurrent)}
}

func normalizeFFprobeMaxConcurrent(n int) int {
	if n <= 0 {
		return 1
	}
	if n > 8 {
		return 8
	}
	return n
}

func (f *FFprobeService) SetMaxConcurrent(n int) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.limiter = make(chan struct{}, normalizeFFprobeMaxConcurrent(n))
}

// ProbeResult is the subset of ffprobe output consumed by the scanner.
type ProbeResult struct {
	DurationSec int
	Width       int
	Height      int
	VideoCodec  string
	AudioCodec  string
	Container   string
}

// Probe runs ffprobe against path and returns a typed result. A 30s timeout
// is applied so a single broken file does not hang the scanner.
func (f *FFprobeService) Probe(ctx context.Context, path string) (*ProbeResult, error) {
	if f == nil {
		return nil, errors.New("ffprobe service nil")
	}
	token, err := f.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer f.release(token)
	if bin, err := resolveLocalExecutable(f.cfg.App.FFprobePath, "ffprobe"); err == nil {
		f.cfg.App.FFprobePath = bin
		probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		cmd := exec.CommandContext(probeCtx, bin, // #nosec G204 -- bin is resolved by resolveLocalExecutable before execution.
			"-v", "error",
			"-print_format", "json",
			"-show_format",
			"-show_streams",
			path,
		)
		out, err := cmd.Output()
		if err == nil {
			return parseProbeJSON(out)
		}
		if f.log != nil {
			f.log.Debug("ffprobe failed, trying ffmpeg fallback", zap.String("path", path), zap.Error(err))
		}
	}
	return f.probeWithFFmpeg(ctx, path)
}

// ProbeHTTP runs ffprobe against a remote HTTP(S) media URL. Headers are
// passed to ffprobe/ffmpeg so WebDAV/OpenList/115 links that require cookies,
// authorization, or a provider-specific User-Agent can still expose stream
// metadata without downloading the whole file.
func (f *FFprobeService) ProbeHTTP(ctx context.Context, rawURL string, headers map[string]string) (*ProbeResult, error) {
	if f == nil {
		return nil, errors.New("ffprobe service nil")
	}
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, errors.New("empty probe url")
	}
	token, err := f.acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer f.release(token)
	headerText := ffmpegHeaderText(headers)
	if bin, err := resolveLocalExecutable(f.cfg.App.FFprobePath, "ffprobe"); err == nil {
		f.cfg.App.FFprobePath = bin
		probeCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		args := []string{"-v", "error"}
		if headerText != "" {
			args = append(args, "-headers", headerText)
		}
		args = append(args, "-print_format", "json", "-show_format", "-show_streams", rawURL)
		cmd := exec.CommandContext(probeCtx, bin, args...) // #nosec G204 -- bin is resolved by resolveLocalExecutable before execution.
		out, err := cmd.Output()
		if err == nil {
			return parseProbeJSON(out)
		}
		if f.log != nil {
			f.log.Debug("remote ffprobe failed, trying ffmpeg fallback", zap.Error(err))
		}
	}
	return f.probeHTTPWithFFmpeg(ctx, rawURL, headerText)
}

func (f *FFprobeService) acquire(ctx context.Context) (chan struct{}, error) {
	f.mu.RLock()
	limiter := f.limiter
	f.mu.RUnlock()
	if limiter == nil {
		return nil, nil
	}
	select {
	case limiter <- struct{}{}:
		return limiter, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (f *FFprobeService) release(limiter chan struct{}) {
	if limiter == nil {
		return
	}
	select {
	case <-limiter:
	default:
	}
}

func (f *FFprobeService) probeWithFFmpeg(ctx context.Context, path string) (*ProbeResult, error) {
	bin, err := resolveLocalExecutable(f.cfg.App.FFmpegPath, "ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffprobe/ffmpeg unavailable: %w", err)
	}
	f.cfg.App.FFmpegPath = bin
	out, _ := commandOutput(ctx, 30*time.Second, bin, "-hide_banner", "-i", path)
	res := parseFFmpegProbeText(string(out))
	if res.VideoCodec == "" && res.AudioCodec == "" && res.DurationSec == 0 {
		return nil, fmt.Errorf("ffmpeg probe %s: no stream metadata parsed", path)
	}
	return res, nil
}

func (f *FFprobeService) probeHTTPWithFFmpeg(ctx context.Context, rawURL, headerText string) (*ProbeResult, error) {
	bin, err := resolveLocalExecutable(f.cfg.App.FFmpegPath, "ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("ffprobe/ffmpeg unavailable: %w", err)
	}
	f.cfg.App.FFmpegPath = bin
	args := []string{"-hide_banner"}
	if headerText != "" {
		args = append(args, "-headers", headerText)
	}
	args = append(args, "-i", rawURL)
	out, _ := commandOutput(ctx, 30*time.Second, bin, args...)
	res := parseFFmpegProbeText(string(out))
	if res.VideoCodec == "" && res.AudioCodec == "" && res.DurationSec == 0 {
		return nil, fmt.Errorf("remote ffmpeg probe: no stream metadata parsed")
	}
	return res, nil
}

func ffmpegHeaderText(headers map[string]string) string {
	if len(headers) == 0 {
		return ""
	}
	var b strings.Builder
	for k, v := range headers {
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if k == "" || strings.ContainsAny(k, "\r\n") || strings.ContainsAny(v, "\r\n") {
			continue
		}
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\r\n")
	}
	return b.String()
}
