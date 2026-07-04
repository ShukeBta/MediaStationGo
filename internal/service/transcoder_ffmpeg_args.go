package service

import (
	"fmt"
	"strconv"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

type ffmpegArgSettings struct {
	encoder        string
	bitrate        string
	maxrate        string
	bufsize        string
	preset         string
	height         int
	segmentSeconds int
	realtime       bool
	threads        int
	vaapiDevice    string
}

type ffmpegVideoPlan struct {
	preInput string
	filter   string
	codec    string
	preset   string
}

// buildFFmpegArgs assembles the ffmpeg command line for the configured
// encoder. The function is package-level so the unit test can pin its
// behaviour without spawning a real ffmpeg process.
func buildFFmpegArgs(cfg *config.Config, source, playlist, segments string) []string {
	settings := ffmpegArgSettingsFromConfig(cfg)
	video := ffmpegVideoPlanForSettings(settings)

	args := baseFFmpegArgs(video.preInput, settings.realtime)
	args = appendInputAndVideoArgs(args, source, settings, video)
	args = appendOutputHLSArgs(args, settings, segments, playlist)
	return args
}

func ffmpegArgSettingsFromConfig(cfg *config.Config) ffmpegArgSettings {
	settings := ffmpegArgSettings{
		bitrate:        ffmpegDefaultString(cfg.Transcoder.VideoBitrate, "1500k"),
		maxrate:        ffmpegDefaultString(cfg.Transcoder.MaxRate, "1800k"),
		bufsize:        ffmpegDefaultString(cfg.Transcoder.BufSize, "3000k"),
		preset:         ffmpegDefaultString(cfg.Transcoder.Preset, "veryfast"),
		height:         cfg.Transcoder.MaxHeight,
		segmentSeconds: cfg.Transcoder.SegmentSeconds,
		realtime:       cfg.Transcoder.Realtime,
		threads:        cfg.Transcoder.Threads,
		vaapiDevice:    ffmpegDefaultString(cfg.App.VAAPIDevice, "/dev/dri/renderD128"),
	}
	if cfg.Transcoder.HardwareAccel {
		settings.encoder = normalizedHardwareEncoder(cfg.Transcoder.Encoder)
	}
	if settings.height <= 0 {
		settings.height = 720
	}
	if settings.segmentSeconds <= 0 {
		settings.segmentSeconds = 4
	}
	return settings
}

func ffmpegDefaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func ffmpegVideoPlanForSettings(settings ffmpegArgSettings) ffmpegVideoPlan {
	switch settings.encoder {
	case "nvenc":
		return ffmpegVideoPlan{
			preInput: "-hwaccel cuda -hwaccel_output_format cuda",
			filter:   fmt.Sprintf("scale_cuda=-2:min(%d\\,ih)", settings.height),
			codec:    "h264_nvenc",
			preset:   "p4",
		}
	case "qsv":
		return ffmpegVideoPlan{
			preInput: "-hwaccel qsv -hwaccel_output_format qsv",
			filter:   fmt.Sprintf("scale_qsv=-1:min(%d\\,ih)", settings.height),
			codec:    "h264_qsv",
			preset:   settings.preset,
		}
	case "vaapi":
		return ffmpegVideoPlan{
			preInput: fmt.Sprintf("-hwaccel vaapi -vaapi_device %s -hwaccel_output_format vaapi", settings.vaapiDevice),
			filter:   fmt.Sprintf("scale_vaapi=-2:min(%d\\,ih),format=nv12|vaapi,hwupload", settings.height),
			codec:    "h264_vaapi",
		}
	default:
		return ffmpegVideoPlan{
			filter: fmt.Sprintf("scale=-2:min(%d\\,ih)", settings.height),
			codec:  "libx264",
			preset: settings.preset,
		}
	}
}

func baseFFmpegArgs(preInput string, realtime bool) []string {
	args := []string{"-y", "-hide_banner", "-nostdin", "-fflags", "+genpts"}
	args = append(args, splitNonEmptyArgs(preInput)...)
	if realtime {
		args = append(args, "-re")
	}
	return args
}

func appendInputAndVideoArgs(args []string, source string, settings ffmpegArgSettings, video ffmpegVideoPlan) []string {
	args = append(args, "-i", source, "-map", "0:v:0?", "-map", "0:a:0?", "-vf", video.filter, "-c:v", video.codec)
	if settings.threads > 0 && video.codec == "libx264" {
		args = append(args, "-threads", strconv.Itoa(settings.threads))
	}
	if video.preset != "" {
		args = append(args, "-preset", video.preset)
	}
	return args
}

func appendOutputHLSArgs(args []string, settings ffmpegArgSettings, segments, playlist string) []string {
	return append(args,
		"-pix_fmt", "yuv420p",
		"-b:v", settings.bitrate,
		"-maxrate", settings.maxrate,
		"-bufsize", settings.bufsize,
		"-c:a", "aac",
		"-ar", "48000",
		"-b:a", "128k",
		"-ac", "2",
		"-force_key_frames", fmt.Sprintf("expr:gte(t,n_forced*%d)", settings.segmentSeconds),
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", settings.segmentSeconds),
		"-hls_list_size", "0",
		"-hls_segment_filename", segments,
		playlist,
	)
}

// splitNonEmptyArgs mirrors the old whitespace split for pre-input flags.
func splitNonEmptyArgs(s string) []string {
	if s == "" {
		return nil
	}
	out := make([]string, 0, 4)
	field := make([]rune, 0, 16)
	flush := func() {
		if len(field) > 0 {
			out = append(out, string(field))
			field = field[:0]
		}
	}
	for _, r := range s {
		if r == ' ' || r == '\t' {
			flush()
			continue
		}
		field = append(field, r)
	}
	flush()
	return out
}
