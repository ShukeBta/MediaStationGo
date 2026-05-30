package service

import (
	"context"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func ApplyRuntimeSettings(ctx context.Context, cfg *config.Config, repos *repository.Container, log *zap.Logger) {
	if cfg == nil || repos == nil || repos.Setting == nil {
		return
	}
	rows, err := repos.Setting.All(ctx)
	if err != nil {
		if log != nil {
			log.Warn("load runtime settings failed", zap.Error(err))
		}
		return
	}
	for _, row := range rows {
		ApplyRuntimeSetting(cfg, row.Key, row.Value)
	}
}

func ApplyRuntimeSetting(cfg *config.Config, key, value string) {
	if cfg == nil {
		return
	}
	value = strings.TrimSpace(value)
	switch key {
	case "ffmpeg.path", "app.ffmpeg_path":
		cfg.App.FFmpegPath = value
	case "ffprobe.path", "app.ffprobe_path":
		cfg.App.FFprobePath = value
	case "transcode.enabled", "transcoder.enabled":
		cfg.Transcoder.Enabled = parseBoolSetting(value, true)
	case "transcode.hw_enabled", "transcoder.hardware_accel":
		cfg.Transcoder.HardwareAccel = parseBoolSetting(value, false)
	case "transcode.hw_accel", "transcoder.encoder":
		switch value {
		case "", "auto", "none", "software":
			cfg.Transcoder.Encoder = ""
		case "nvenc", "qsv", "vaapi":
			cfg.Transcoder.Encoder = value
		}
	case "transcode.max_height", "transcoder.max_height":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.Transcoder.MaxHeight = n
		}
	case "transcode.max_jobs", "transcoder.max_concurrent":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.Transcoder.MaxConcurrent = n
		}
	case "transcode.realtime", "transcoder.realtime":
		cfg.Transcoder.Realtime = parseBoolSetting(value, true)
	case "transcode.threads", "transcoder.threads":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.Transcoder.Threads = n
		}
	case "transcode.idle_timeout_seconds", "transcoder.idle_timeout_seconds":
		if n, err := strconv.Atoi(value); err == nil {
			cfg.Transcoder.IdleTimeoutSeconds = n
		}
	case "transcode.video_bitrate", "transcoder.video_bitrate":
		cfg.Transcoder.VideoBitrate = value
	case "license.server_url":
		cfg.License.ServerURL = value
	case "license.hmac_secret":
		cfg.License.HMACSecret = value
	}
}

func parseBoolSetting(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled", "启用", "开启":
		return true
	case "0", "false", "no", "off", "disabled", "禁用", "关闭":
		return false
	default:
		return fallback
	}
}
