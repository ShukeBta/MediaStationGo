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
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
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
	for _, s := range raw.Streams {
		switch s.CodecType {
		case "video":
			if res.VideoCodec == "" {
				res.VideoCodec = s.CodecName
				res.Width = s.Width
				res.Height = s.Height
			}
		case "audio":
			if res.AudioCodec == "" {
				res.AudioCodec = s.CodecName
			}
		}
	}
	return res, nil
}

var (
	ffmpegDurationRE = regexp.MustCompile(`Duration:\s*(\d+):(\d+):(\d+(?:\.\d+)?)`)
	ffmpegInputRE    = regexp.MustCompile(`Input #\d+,\s*(.+?),\s*from`)
	ffmpegVideoRE    = regexp.MustCompile(`Video:\s*([^,\s]+).*?(\d{2,5})x(\d{2,5})`)
	ffmpegAudioRE    = regexp.MustCompile(`Audio:\s*([^,\s]+)`)
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
	}
	return res
}
