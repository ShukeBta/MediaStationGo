package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
)

func (t *TranscoderService) runFFmpeg(ctx context.Context, job *hlsJob, source string) {
	bin, err := t.resolveFFmpegPath()
	if err != nil {
		t.log.Warn("ffmpeg unavailable", zap.String("media_id", job.mediaID), zap.Error(err))
		t.mu.Lock()
		delete(t.jobs, job.mediaID)
		t.mu.Unlock()
		t.hub.Publish("transcode", map[string]any{
			"media_id": job.mediaID,
			"status":   "error",
			"error":    err.Error(),
		})
		return
	}

	playlist := filepath.Join(job.outputDir, "index.m3u8")
	segments := filepath.Join(job.outputDir, "seg_%05d.ts")

	args := buildFFmpegArgs(t.cfg, source, playlist, segments)

	cmd := exec.CommandContext(ctx, bin, args...) // #nosec G204 -- bin is resolved by resolveFFmpegPath and args are passed without a shell.
	cmd.Stderr = os.Stderr

	t.log.Info("transcode started",
		zap.String("media_id", job.mediaID),
		zap.String("encoder", job.encoder),
		zap.String("source", source),
	)
	t.hub.Publish("transcode", map[string]any{
		"media_id": job.mediaID,
		"encoder":  job.encoder,
		"status":   "started",
	})

	if err := cmd.Run(); err != nil && !errors.Is(ctx.Err(), context.Canceled) {
		t.log.Warn("ffmpeg exited",
			zap.String("media_id", job.mediaID),
			zap.Error(err),
		)
	}

	t.mu.Lock()
	delete(t.jobs, job.mediaID)
	t.mu.Unlock()

	t.hub.Publish("transcode", map[string]any{
		"media_id": job.mediaID,
		"status":   "stopped",
		"duration": time.Since(job.startedAt).Seconds(),
	})
}

func (t *TranscoderService) resolveFFmpegPath() (string, error) {
	var lastErr error
	for _, bin := range executableCandidates(strings.TrimSpace(t.cfg.App.FFmpegPath), "ffmpeg") {
		if err := validateFFmpegForTranscode(context.Background(), bin, t.effectiveEncoder()); err != nil {
			lastErr = err
			continue
		}
		t.cfg.App.FFmpegPath = bin
		return bin, nil
	}
	if lastErr != nil {
		return "", fmt.Errorf("no usable ffmpeg found for HLS transcode: %w", lastErr)
	}
	return "", fmt.Errorf("ffmpeg not found in PATH or common local app directories; configure app.ffmpeg_path to an existing local ffmpeg")
}

func (t *TranscoderService) effectiveEncoder() string {
	if !t.cfg.Transcoder.HardwareAccel {
		return ""
	}
	return normalizedHardwareEncoder(t.cfg.Transcoder.Encoder)
}

func normalizedHardwareEncoder(encoder string) string {
	switch strings.ToLower(strings.TrimSpace(encoder)) {
	case "nvenc", "qsv", "vaapi":
		return strings.ToLower(strings.TrimSpace(encoder))
	default:
		return ""
	}
}

func validateFFmpegForTranscode(ctx context.Context, bin, encoder string) error {
	required := requiredVideoEncoder(encoder)
	out, err := commandOutput(ctx, 8*time.Second, bin, "-hide_banner", "-encoders")
	if err != nil {
		return fmt.Errorf("%s cannot list encoders: %w", bin, err)
	}
	if !hasFFmpegListEntry(string(out), required) {
		return fmt.Errorf("%s does not provide required encoder %s", bin, required)
	}

	out, err = commandOutput(ctx, 8*time.Second, bin, "-hide_banner", "-muxers")
	if err != nil || !hasFFmpegListEntry(string(out), "hls") {
		out, err = commandOutput(ctx, 8*time.Second, bin, "-hide_banner", "-formats")
		if err != nil {
			return fmt.Errorf("%s cannot list muxers/formats: %w", bin, err)
		}
	}
	if !hasFFmpegListEntry(string(out), "hls") {
		return fmt.Errorf("%s does not provide hls muxer", bin)
	}
	return nil
}

func requiredVideoEncoder(encoder string) string {
	switch encoder {
	case "nvenc":
		return "h264_nvenc"
	case "qsv":
		return "h264_qsv"
	case "vaapi":
		return "h264_vaapi"
	default:
		return "libx264"
	}
}

func hasFFmpegListEntry(output, name string) bool {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		for _, field := range fields {
			if field == name {
				return true
			}
		}
	}
	return false
}

// HumanFFmpegProfile is exposed for the admin UI / settings view.
func (t *TranscoderService) HumanFFmpegProfile() string {
	return fmt.Sprintf("ffmpeg=%s, encoder=%s, output=%s",
		t.cfg.App.FFmpegPath, t.cfg.Transcoder.Encoder, filepath.Join(t.cfg.Cache.CacheDir, "hls"))
}
