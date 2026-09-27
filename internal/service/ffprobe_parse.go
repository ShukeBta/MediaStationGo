package service

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// rawProbe mirrors the relevant fields of `ffprobe -show_format -show_streams`.
type rawProbe struct {
	Format struct {
		Duration   string `json:"duration"`
		FormatName string `json:"format_name"`
		BitRate    string `json:"bit_rate"`
	} `json:"format"`
	Streams []struct {
		Index            int    `json:"index"`
		CodecType        string `json:"codec_type"`
		CodecName        string `json:"codec_name"`
		Profile          string `json:"profile"`
		Width            int    `json:"width"`
		Height           int    `json:"height"`
		BitRate          string `json:"bit_rate"`
		AvgFrameRate     string `json:"avg_frame_rate"`
		RFrameRate       string `json:"r_frame_rate"`
		PixelFormat      string `json:"pix_fmt"`
		BitsPerRawSample string `json:"bits_per_raw_sample"`
		BitsPerSample    int    `json:"bits_per_sample"`
		ColorTransfer    string `json:"color_transfer"`
		Channels         int    `json:"channels"`
		ChannelLayout    string `json:"channel_layout"`
		SampleRate       string `json:"sample_rate"`
		SideDataList     []struct {
			SideDataType string `json:"side_data_type"`
		} `json:"side_data_list"`
		Tags struct {
			Language string `json:"language"`
			Title    string `json:"title"`
		} `json:"tags"`
		Disposition struct {
			Default int `json:"default"`
			Forced  int `json:"forced"`
		} `json:"disposition"`
	} `json:"streams"`
}

func parseProbeJSON(data []byte) (*ProbeResult, error) {
	var raw rawProbe
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe json: %w", err)
	}
	res := &ProbeResult{Container: raw.Format.FormatName}
	if d, err := strconv.ParseFloat(raw.Format.Duration, 64); err == nil {
		res.DurationSec = int(d)
	}
	res.BitRate = parseProbeInt64(raw.Format.BitRate)
	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			if res.VideoCodec == "" {
				res.VideoCodec = s.CodecName
				res.Width = s.Width
				res.Height = s.Height
				res.VideoBitRate = parseProbeInt64(s.BitRate)
				res.FrameRate = parseProbeFrameRate(firstNonEmpty(s.AvgFrameRate, s.RFrameRate))
				res.VideoProfile = strings.TrimSpace(s.Profile)
				res.VideoRange = probeVideoRange(s.ColorTransfer, s.SideDataList)
				res.VideoBitDepth = probeVideoBitDepth(s.BitsPerRawSample, s.BitsPerSample, s.PixelFormat)
			}
		case "audio":
			if res.AudioCodec == "" {
				res.AudioCodec = s.CodecName
				res.AudioBitRate = parseProbeInt64(s.BitRate)
				res.AudioChannels = s.Channels
				res.AudioChannelLayout = strings.TrimSpace(s.ChannelLayout)
				res.AudioSampleRate = int(parseProbeInt64(s.SampleRate))
			}
		case "subtitle":
			res.SubtitleStreams = append(res.SubtitleStreams, ProbeSubtitleStream{
				Index:    s.Index,
				Codec:    strings.TrimSpace(s.CodecName),
				Language: strings.TrimSpace(s.Tags.Language),
				Title:    strings.TrimSpace(s.Tags.Title),
				Default:  s.Disposition.Default != 0,
				Forced:   s.Disposition.Forced != 0,
			})
		}
	}
	return res, nil
}

func parseProbeInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return parsed
}

func parseProbeFrameRate(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "0/0" {
		return 0
	}
	parts := strings.SplitN(value, "/", 2)
	if len(parts) == 2 {
		numerator, errN := strconv.ParseFloat(parts[0], 64)
		denominator, errD := strconv.ParseFloat(parts[1], 64)
		if errN == nil && errD == nil && denominator != 0 {
			return numerator / denominator
		}
	}
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

func probeVideoRange(transfer string, sideData []struct {
	SideDataType string `json:"side_data_type"`
}) string {
	for _, item := range sideData {
		value := strings.ToLower(strings.TrimSpace(item.SideDataType))
		if strings.Contains(value, "dovi") || strings.Contains(value, "dolby vision") {
			return "Dolby Vision"
		}
	}
	switch strings.ToLower(strings.TrimSpace(transfer)) {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	case "bt709", "iec61966-2-1", "gamma22", "gamma28":
		return "SDR"
	default:
		return ""
	}
}

func probeVideoBitDepth(raw string, bitsPerSample int, pixelFormat string) int {
	if value, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil && value > 0 {
		return value
	}
	if bitsPerSample > 0 {
		return bitsPerSample
	}
	pixelFormat = strings.ToLower(strings.TrimSpace(pixelFormat))
	match := probeBitDepthRE.FindStringSubmatch(pixelFormat)
	if len(match) == 2 {
		value, _ := strconv.Atoi(match[1])
		return value
	}
	if pixelFormat != "" {
		return 8
	}
	return 0
}

var (
	ffmpegDurationRE = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)
	ffmpegInputRE    = regexp.MustCompile(`Input #\d+,\s*(.+?),\s*from`)
	ffmpegVideoRE    = regexp.MustCompile(`Video:\s*([^,\s]+).*?(\d{2,5})x(\d{2,5})`)
	ffmpegAudioRE    = regexp.MustCompile(`Audio:\s*([^,\s]+)`)
	ffmpegSubtitleRE = regexp.MustCompile(`Stream #\d+:(\d+)(?:\(([^)]+)\))?.*Subtitle:\s*([^,\s]+)`)
	probeBitDepthRE  = regexp.MustCompile(`p0?(10|12|14|16)(?:le|be)?$`)
)

func parseFFmpegProbeText(text string) *ProbeResult {
	res := &ProbeResult{}
	if match := ffmpegInputRE.FindStringSubmatch(text); len(match) == 2 {
		res.Container = strings.TrimSpace(match[1])
	}
	if match := ffmpegDurationRE.FindStringSubmatch(text); len(match) == 4 {
		hours, _ := strconv.Atoi(match[1])
		minutes, _ := strconv.Atoi(match[2])
		seconds, _ := strconv.ParseFloat(match[3], 64)
		res.DurationSec = hours*3600 + minutes*60 + int(seconds)
	}
	for _, line := range strings.Split(text, "\n") {
		if res.VideoCodec == "" {
			if match := ffmpegVideoRE.FindStringSubmatch(line); len(match) == 4 {
				res.VideoCodec = strings.TrimSpace(match[1])
				res.Width, _ = strconv.Atoi(match[2])
				res.Height, _ = strconv.Atoi(match[3])
			}
		}
		if res.AudioCodec == "" {
			if match := ffmpegAudioRE.FindStringSubmatch(line); len(match) == 2 {
				res.AudioCodec = strings.TrimSpace(match[1])
			}
		}
		if match := ffmpegSubtitleRE.FindStringSubmatch(line); len(match) == 4 {
			index, _ := strconv.Atoi(match[1])
			lowered := strings.ToLower(line)
			res.SubtitleStreams = append(res.SubtitleStreams, ProbeSubtitleStream{
				Index:    index,
				Language: strings.TrimSpace(match[2]),
				Codec:    strings.TrimSpace(match[3]),
				Default:  strings.Contains(lowered, "(default)"),
				Forced:   strings.Contains(lowered, "(forced)"),
			})
		}
	}
	return res
}
