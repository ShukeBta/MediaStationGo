package service

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// rawProbe mirrors the relevant fields of `ffprobe -show_format -show_streams -show_chapters`.
type rawProbe struct {
	Format struct {
		Duration       string            `json:"duration"`
		FormatName     string            `json:"format_name"`
		FormatLongName string            `json:"format_long_name"`
		StartTime      string            `json:"start_time"`
		Size           string            `json:"size"`
		BitRate        string            `json:"bit_rate"`
		ProbeScore     int               `json:"probe_score"`
		Tags           map[string]string `json:"tags"`
	} `json:"format"`
	Streams  []rawProbeStream `json:"streams"`
	Chapters []struct {
		ID        int               `json:"id"`
		StartTime string            `json:"start_time"`
		EndTime   string            `json:"end_time"`
		Tags      map[string]string `json:"tags"`
	} `json:"chapters"`
}

type rawProbeSideData = struct {
	SideDataType string `json:"side_data_type"`
}

type rawProbeStream struct {
	Index              int                `json:"index"`
	CodecType          string             `json:"codec_type"`
	CodecName          string             `json:"codec_name"`
	CodecLongName      string             `json:"codec_long_name"`
	Profile            string             `json:"profile"`
	Level              int                `json:"level"`
	TimeBase           string             `json:"time_base"`
	StartTime          string             `json:"start_time"`
	Duration           string             `json:"duration"`
	Width              int                `json:"width"`
	Height             int                `json:"height"`
	SampleAspectRatio  string             `json:"sample_aspect_ratio"`
	DisplayAspectRatio string             `json:"display_aspect_ratio"`
	BitRate            string             `json:"bit_rate"`
	AvgFrameRate       string             `json:"avg_frame_rate"`
	RFrameRate         string             `json:"r_frame_rate"`
	PixelFormat        string             `json:"pix_fmt"`
	BitsPerRawSample   string             `json:"bits_per_raw_sample"`
	BitsPerSample      int                `json:"bits_per_sample"`
	ColorRange         string             `json:"color_range"`
	ColorSpace         string             `json:"color_space"`
	ColorTransfer      string             `json:"color_transfer"`
	ColorPrimaries     string             `json:"color_primaries"`
	SampleFormat       string             `json:"sample_fmt"`
	Channels           int                `json:"channels"`
	ChannelLayout      string             `json:"channel_layout"`
	SampleRate         string             `json:"sample_rate"`
	SideDataList       []rawProbeSideData `json:"side_data_list"`
	Tags               map[string]string  `json:"tags"`
	Disposition        struct {
		Default         int `json:"default"`
		Dub             int `json:"dub"`
		Original        int `json:"original"`
		Comment         int `json:"comment"`
		Forced          int `json:"forced"`
		HearingImpaired int `json:"hearing_impaired"`
		VisualImpaired  int `json:"visual_impaired"`
		AttachedPic     int `json:"attached_pic"`
	} `json:"disposition"`
}

func parseProbeJSON(data []byte) (*ProbeResult, error) {
	var raw rawProbe
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse ffprobe json: %w", err)
	}
	res := &ProbeResult{Container: raw.Format.FormatName}
	res.DurationSec = int(parseProbeFloat(raw.Format.Duration))
	res.BitRate = parseProbeInt64(raw.Format.BitRate)
	res.Document = probeDocumentFromRaw(&raw)
	for _, s := range raw.Streams {
		if s.Disposition.AttachedPic != 0 {
			// 封面图(attached_pic)不是可播放视频轨,不能覆盖真实视频参数。
			continue
		}
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
				Language: strings.TrimSpace(s.Tags["language"]),
				Title:    strings.TrimSpace(s.Tags["title"]),
				Default:  s.Disposition.Default != 0,
				Forced:   s.Disposition.Forced != 0,
			})
		}
	}
	return res, nil
}

// probeDocumentFromRaw 把 ffprobe 原始输出裁剪为可持久化的白名单文档。
func probeDocumentFromRaw(raw *rawProbe) *ProbeDocument {
	if raw == nil {
		return nil
	}
	doc := &ProbeDocument{
		SchemaVersion: ProbeDocumentSchemaVersion,
		Format: ProbeFormat{
			Name:       strings.TrimSpace(raw.Format.FormatName),
			LongName:   strings.TrimSpace(raw.Format.FormatLongName),
			StartTime:  parseProbeFloat(raw.Format.StartTime),
			Duration:   parseProbeFloat(raw.Format.Duration),
			Size:       parseProbeInt64(raw.Format.Size),
			BitRate:    parseProbeInt64(raw.Format.BitRate),
			ProbeScore: raw.Format.ProbeScore,
			Tags:       safeProbeTags(raw.Format.Tags),
		},
		Streams: make([]ProbeStream, 0, len(raw.Streams)),
	}
	seen := make(map[int]struct{}, len(raw.Streams))
	for _, stream := range raw.Streams {
		if stream.Disposition.AttachedPic != 0 || stream.Index < 0 {
			continue
		}
		if stream.CodecType != "video" && stream.CodecType != "audio" && stream.CodecType != "subtitle" {
			continue
		}
		if _, dup := seen[stream.Index]; dup {
			continue
		}
		seen[stream.Index] = struct{}{}
		doc.Streams = append(doc.Streams, normalizeProbeStream(stream))
	}
	for _, chapter := range raw.Chapters {
		doc.Chapters = append(doc.Chapters, ProbeChapter{
			ID:        chapter.ID,
			StartTime: parseProbeFloat(chapter.StartTime),
			EndTime:   parseProbeFloat(chapter.EndTime),
			Title:     strings.TrimSpace(chapter.Tags["title"]),
		})
	}
	return doc
}

func normalizeProbeStream(raw rawProbeStream) ProbeStream {
	sideData := make([]string, 0, len(raw.SideDataList))
	for _, side := range raw.SideDataList {
		if value := strings.TrimSpace(side.SideDataType); value != "" {
			sideData = append(sideData, value)
		}
	}
	return ProbeStream{
		Index:              raw.Index,
		CodecType:          raw.CodecType,
		CodecName:          strings.TrimSpace(raw.CodecName),
		CodecLongName:      strings.TrimSpace(raw.CodecLongName),
		Profile:            strings.TrimSpace(raw.Profile),
		Level:              raw.Level,
		TimeBase:           raw.TimeBase,
		StartTime:          parseProbeFloat(raw.StartTime),
		Duration:           parseProbeFloat(raw.Duration),
		BitRate:            parseProbeInt64(raw.BitRate),
		Width:              raw.Width,
		Height:             raw.Height,
		SampleAspectRatio:  raw.SampleAspectRatio,
		DisplayAspectRatio: raw.DisplayAspectRatio,
		PixelFormat:        raw.PixelFormat,
		BitDepth:           probeStreamBitDepth(raw),
		ColorRange:         raw.ColorRange,
		ColorSpace:         raw.ColorSpace,
		ColorTransfer:      raw.ColorTransfer,
		ColorPrimaries:     raw.ColorPrimaries,
		AverageFrameRate:   raw.AvgFrameRate,
		RealFrameRate:      raw.RFrameRate,
		SampleFormat:       raw.SampleFormat,
		SampleRate:         int(parseProbeInt64(raw.SampleRate)),
		Channels:           raw.Channels,
		ChannelLayout:      strings.TrimSpace(raw.ChannelLayout),
		BitsPerSample:      raw.BitsPerSample,
		Tags:               safeProbeTags(raw.Tags),
		SideData:           sideData,
		Disposition: ProbeDisposition{
			Default:         raw.Disposition.Default != 0,
			Dub:             raw.Disposition.Dub != 0,
			Original:        raw.Disposition.Original != 0,
			Comment:         raw.Disposition.Comment != 0,
			Forced:          raw.Disposition.Forced != 0,
			HearingImpaired: raw.Disposition.HearingImpaired != 0,
			VisualImpaired:  raw.Disposition.VisualImpaired != 0,
		},
	}
}

func probeStreamBitDepth(raw rawProbeStream) int {
	if raw.CodecType != "video" {
		return 0
	}
	return probeVideoBitDepth(raw.BitsPerRawSample, raw.BitsPerSample, raw.PixelFormat)
}

// safeProbeTags 只保留语言和标题,丢弃 encoder/handler 等可能泄露来源的标签。
func safeProbeTags(tags map[string]string) ProbeTags {
	return ProbeTags{
		Language: strings.TrimSpace(tags["language"]),
		Title:    strings.TrimSpace(tags["title"]),
	}
}

func parseProbeFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if math.IsNaN(parsed) || math.IsInf(parsed, 0) {
		return 0
	}
	return parsed
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
		if errN == nil && errD == nil && denominator > 0 && numerator > 0 {
			result := numerator / denominator
			if !math.IsNaN(result) && !math.IsInf(result, 0) {
				return result
			}
		}
		return 0
	}
	return parseProbeFloat(value)
}

func probeVideoRange(transfer string, sideData []rawProbeSideData) string {
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
