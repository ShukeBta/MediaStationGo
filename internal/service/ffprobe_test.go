package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestFFprobeServiceDefaultsToSingleConcurrentProbe(t *testing.T) {
	svc := NewFFprobeService(&config.Config{}, zap.NewNop())
	if got := cap(svc.limiter); got != 1 {
		t.Fatalf("limiter capacity = %d, want 1", got)
	}
}

func TestFFprobeServiceClampsConfiguredConcurrency(t *testing.T) {
	cfg := &config.Config{}
	cfg.App.FFprobeMaxConcurrent = 3
	svc := NewFFprobeService(cfg, zap.NewNop())
	if got := cap(svc.limiter); got != 3 {
		t.Fatalf("limiter capacity = %d, want 3", got)
	}

	cfg.App.FFprobeMaxConcurrent = 99
	svc = NewFFprobeService(cfg, zap.NewNop())
	if got := cap(svc.limiter); got != 8 {
		t.Fatalf("limiter capacity = %d, want max clamp 8", got)
	}
}

func TestFFprobeAcquireHonorsContextWhenLimitReached(t *testing.T) {
	svc := &FFprobeService{limiter: make(chan struct{}, 1)}
	token, err := svc.acquire(t.Context())
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	if _, err := svc.acquire(ctx); err == nil {
		t.Fatal("second acquire should block until context deadline")
	}
	svc.release(token)
	token, err = svc.acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	svc.release(token)
}

func TestFFprobeSetMaxConcurrentHotSwapsLimiter(t *testing.T) {
	svc := NewFFprobeService(&config.Config{}, zap.NewNop())
	firstToken, err := svc.acquire(t.Context())
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	svc.SetMaxConcurrent(2)
	secondToken, err := svc.acquire(t.Context())
	if err != nil {
		t.Fatalf("acquire after resize: %v", err)
	}
	thirdToken, err := svc.acquire(t.Context())
	if err != nil {
		t.Fatalf("second acquire after resize: %v", err)
	}
	svc.release(firstToken)
	svc.release(secondToken)
	svc.release(thirdToken)
}

func TestApplyRuntimeSettingFFprobeMaxConcurrent(t *testing.T) {
	cfg := &config.Config{}
	ApplyRuntimeSetting(cfg, "ffprobe.max_concurrent", "4")
	if cfg.App.FFprobeMaxConcurrent != 4 {
		t.Fatalf("FFprobeMaxConcurrent = %d, want 4", cfg.App.FFprobeMaxConcurrent)
	}
	ApplyRuntimeSetting(cfg, "ffprobe.max_concurrent", "99")
	if cfg.App.FFprobeMaxConcurrent != 8 {
		t.Fatalf("FFprobeMaxConcurrent = %d, want clamp 8", cfg.App.FFprobeMaxConcurrent)
	}
	ApplyRuntimeSetting(cfg, "ffprobe.max_concurrent", "0")
	if cfg.App.FFprobeMaxConcurrent != 1 {
		t.Fatalf("FFprobeMaxConcurrent = %d, want clamp 1", cfg.App.FFprobeMaxConcurrent)
	}
}

func TestParseProbeJSONExtractsPrimaryStreams(t *testing.T) {
	got, err := parseProbeJSON([]byte(`{
		"format": {"duration": "125.900000", "format_name": "matroska,webm", "bit_rate": "24000000"},
		"streams": [
			{"codec_type": "video", "codec_name": "hevc", "profile": "Main 10", "width": 3840, "height": 2160, "bit_rate": "22000000", "avg_frame_rate": "24000/1001", "pix_fmt": "yuv420p10le", "color_transfer": "smpte2084"},
			{"codec_type": "audio", "codec_name": "eac3", "bit_rate": "768000", "channels": 6, "channel_layout": "5.1(side)", "sample_rate": "48000"},
			{"index": 2, "codec_type": "subtitle", "codec_name": "ass", "tags": {"language": "zh-Hans", "title": "简体中文"}, "disposition": {"default": 1, "forced": 0}},
			{"codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080}
		]
	}`))
	if err != nil {
		t.Fatalf("parseProbeJSON: %v", err)
	}
	if got.DurationSec != 125 || got.Container != "matroska,webm" || got.VideoCodec != "hevc" || got.AudioCodec != "eac3" || got.Width != 3840 || got.Height != 2160 {
		t.Fatalf("parsed probe = %+v", got)
	}
	if got.BitRate != 24000000 || got.VideoBitRate != 22000000 || got.FrameRate < 23.97 || got.FrameRate > 23.99 || got.VideoProfile != "Main 10" || got.VideoRange != "HDR10" || got.VideoBitDepth != 10 {
		t.Fatalf("parsed video details = %+v", got)
	}
	if got.AudioBitRate != 768000 || got.AudioChannels != 6 || got.AudioChannelLayout != "5.1(side)" || got.AudioSampleRate != 48000 {
		t.Fatalf("parsed audio details = %+v", got)
	}
	if len(got.SubtitleStreams) != 1 || got.SubtitleStreams[0].Index != 2 || got.SubtitleStreams[0].Language != "zh-Hans" || got.SubtitleStreams[0].Title != "简体中文" || !got.SubtitleStreams[0].Default {
		t.Fatalf("parsed subtitle streams = %+v", got.SubtitleStreams)
	}
}

func TestParseFFmpegProbeTextExtractsFallbackMetadata(t *testing.T) {
	got := parseFFmpegProbeText(`Input #0, matroska,webm, from 'movie.mkv':
  Duration: 01:02:03.45, start: 0.000000, bitrate: N/A
  Stream #0:0: Video: h264 (High), yuv420p(progressive), 1920x804
	  Stream #0:1: Audio: aac, 48000 Hz, stereo
	  Stream #0:2(chi): Subtitle: subrip (srt) (default)`)
	if got.DurationSec != 3723 || got.Container != "matroska,webm" || got.VideoCodec != "h264" || got.AudioCodec != "aac" || got.Width != 1920 || got.Height != 804 {
		t.Fatalf("parsed ffmpeg text = %+v", got)
	}
	if len(got.SubtitleStreams) != 1 || got.SubtitleStreams[0].Index != 2 || got.SubtitleStreams[0].Language != "chi" || got.SubtitleStreams[0].Codec != "subrip" || !got.SubtitleStreams[0].Default {
		t.Fatalf("parsed ffmpeg subtitle streams = %+v", got.SubtitleStreams)
	}
}
