package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ProbeDocumentSchemaVersion 是持久化轨道文档的结构版本;字段白名单变化时递增,
// 旧版本文档在读取时被忽略,由下次探测覆盖。
const ProbeDocumentSchemaVersion = 1

// ProbeDocument 是允许持久化的 ffprobe 字段白名单,不包含输入路径、URL 或请求凭据。
type ProbeDocument struct {
	SchemaVersion int            `json:"schema_version"`
	Format        ProbeFormat    `json:"format"`
	Streams       []ProbeStream  `json:"streams"`
	Chapters      []ProbeChapter `json:"chapters,omitempty"`
}

type ProbeFormat struct {
	Name       string    `json:"name,omitempty"`
	LongName   string    `json:"long_name,omitempty"`
	StartTime  float64   `json:"start_time,omitempty"`
	Duration   float64   `json:"duration,omitempty"`
	Size       int64     `json:"size,omitempty"`
	BitRate    int64     `json:"bit_rate,omitempty"`
	ProbeScore int       `json:"probe_score,omitempty"`
	Tags       ProbeTags `json:"tags,omitempty"`
}

// ProbeStream.Index 保留 ffprobe 的绝对 stream index,选轨不得改用数组位置。
type ProbeStream struct {
	Index              int              `json:"index"`
	CodecType          string           `json:"codec_type"`
	CodecName          string           `json:"codec_name,omitempty"`
	CodecLongName      string           `json:"codec_long_name,omitempty"`
	Profile            string           `json:"profile,omitempty"`
	Level              int              `json:"level,omitempty"`
	TimeBase           string           `json:"time_base,omitempty"`
	StartTime          float64          `json:"start_time,omitempty"`
	Duration           float64          `json:"duration,omitempty"`
	BitRate            int64            `json:"bit_rate,omitempty"`
	Width              int              `json:"width,omitempty"`
	Height             int              `json:"height,omitempty"`
	SampleAspectRatio  string           `json:"sample_aspect_ratio,omitempty"`
	DisplayAspectRatio string           `json:"display_aspect_ratio,omitempty"`
	PixelFormat        string           `json:"pixel_format,omitempty"`
	BitDepth           int              `json:"bit_depth,omitempty"`
	ColorRange         string           `json:"color_range,omitempty"`
	ColorSpace         string           `json:"color_space,omitempty"`
	ColorTransfer      string           `json:"color_transfer,omitempty"`
	ColorPrimaries     string           `json:"color_primaries,omitempty"`
	AverageFrameRate   string           `json:"average_frame_rate,omitempty"`
	RealFrameRate      string           `json:"real_frame_rate,omitempty"`
	SampleFormat       string           `json:"sample_format,omitempty"`
	SampleRate         int              `json:"sample_rate,omitempty"`
	Channels           int              `json:"channels,omitempty"`
	ChannelLayout      string           `json:"channel_layout,omitempty"`
	BitsPerSample      int              `json:"bits_per_sample,omitempty"`
	Tags               ProbeTags        `json:"tags,omitempty"`
	Disposition        ProbeDisposition `json:"disposition,omitempty"`
	SideData           []string         `json:"side_data,omitempty"`
}

type ProbeDisposition struct {
	Default         bool `json:"default,omitempty"`
	Dub             bool `json:"dub,omitempty"`
	Original        bool `json:"original,omitempty"`
	Comment         bool `json:"comment,omitempty"`
	Forced          bool `json:"forced,omitempty"`
	HearingImpaired bool `json:"hearing_impaired,omitempty"`
	VisualImpaired  bool `json:"visual_impaired,omitempty"`
	AttachedPic     bool `json:"attached_pic,omitempty"`
}

type ProbeTags struct {
	Language string `json:"language,omitempty"`
	Title    string `json:"title,omitempty"`
}

type ProbeChapter struct {
	ID        int     `json:"id"`
	StartTime float64 `json:"start_time,omitempty"`
	EndTime   float64 `json:"end_time,omitempty"`
	Title     string  `json:"title,omitempty"`
}

func MarshalProbeDocument(doc *ProbeDocument) (string, error) {
	if err := validateProbeDocument(doc); err != nil {
		return "", err
	}
	b, err := json.Marshal(doc)
	return string(b), err
}

func UnmarshalProbeDocument(data string, schemaVersion int) (*ProbeDocument, error) {
	if schemaVersion != ProbeDocumentSchemaVersion {
		return nil, errors.New("unsupported probe document version")
	}
	var doc ProbeDocument
	if err := json.Unmarshal([]byte(data), &doc); err != nil {
		return nil, err
	}
	if doc.SchemaVersion != schemaVersion {
		return nil, errors.New("probe document version mismatch")
	}
	if err := validateProbeDocument(&doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func validateProbeDocument(doc *ProbeDocument) error {
	if doc == nil || doc.SchemaVersion != ProbeDocumentSchemaVersion {
		return errors.New("invalid probe document version")
	}
	seen := make(map[int]struct{}, len(doc.Streams))
	for _, stream := range doc.Streams {
		if stream.Index < 0 {
			return fmt.Errorf("invalid probe stream index %d", stream.Index)
		}
		if _, ok := seen[stream.Index]; ok {
			return fmt.Errorf("duplicate probe stream index %d", stream.Index)
		}
		seen[stream.Index] = struct{}{}
		if stream.Disposition.AttachedPic {
			return fmt.Errorf("attached picture stream %d is not persistable", stream.Index)
		}
		switch stream.CodecType {
		case "video", "audio", "subtitle":
		default:
			return fmt.Errorf("unsupported probe stream type %q", stream.CodecType)
		}
	}
	return nil
}

// probeDocumentHasTracks 表示文档至少含一条音视频轨,空文档不值得持久化。
func probeDocumentHasTracks(doc *ProbeDocument) bool {
	if doc == nil {
		return false
	}
	for _, stream := range doc.Streams {
		if stream.CodecType == "video" || stream.CodecType == "audio" {
			return true
		}
	}
	return false
}

func projectProbeTracks(doc *ProbeDocument) []model.MediaTrack {
	if doc == nil {
		return nil
	}
	tracks := make([]model.MediaTrack, 0, len(doc.Streams))
	for _, stream := range doc.Streams {
		if track, ok := projectProbeTrack(stream); ok {
			tracks = append(tracks, track)
		}
	}
	return tracks
}

func projectProbeTrack(stream ProbeStream) (model.MediaTrack, bool) {
	if stream.CodecType != "video" && stream.CodecType != "audio" && stream.CodecType != "subtitle" {
		return model.MediaTrack{}, false
	}
	track := model.MediaTrack{
		Index:             stream.Index,
		Type:              stream.CodecType,
		Codec:             stream.CodecName,
		Profile:           stream.Profile,
		Level:             stream.Level,
		TimeBase:          stream.TimeBase,
		Language:          stream.Tags.Language,
		DisplayLanguage:   probeDisplayLanguage(stream.Tags.Language),
		Title:             stream.Tags.Title,
		DisplayTitle:      probeStreamDisplayTitle(stream),
		BitRate:           stream.BitRate,
		IsDefault:         stream.Disposition.Default,
		IsForced:          stream.Disposition.Forced,
		IsHearingImpaired: stream.Disposition.HearingImpaired,
		IsVisualImpaired:  stream.Disposition.VisualImpaired,
	}
	if track.BitRate < 0 {
		track.BitRate = 0
	}
	switch stream.CodecType {
	case "video":
		track.Width = stream.Width
		track.Height = stream.Height
		track.AspectRatio = firstNonEmpty(stream.DisplayAspectRatio, stream.SampleAspectRatio)
		track.PixelFormat = stream.PixelFormat
		track.BitDepth = stream.BitDepth
		track.ColorRange = stream.ColorRange
		track.ColorSpace = stream.ColorSpace
		track.ColorTransfer = stream.ColorTransfer
		track.ColorPrimaries = stream.ColorPrimaries
		track.VideoRange = probeDocVideoRange(stream)
		if rate := parseProbeFrameRate(stream.AverageFrameRate); rate > 0 {
			track.AverageFrameRate = rate
		}
		if rate := parseProbeFrameRate(stream.RealFrameRate); rate > 0 {
			track.RealFrameRate = rate
		}
	case "audio":
		track.Channels = stream.Channels
		track.SampleRate = stream.SampleRate
		track.ChannelLayout = stream.ChannelLayout
		track.SampleFormat = stream.SampleFormat
		track.BitsPerSample = stream.BitsPerSample
	default:
		track.IsTextSubtitle = isTextSubtitleCodec(stream.CodecName)
	}
	return track, true
}

func probeDisplayLanguage(code string) string {
	code = strings.TrimSpace(code)
	if code == "" || strings.EqualFold(code, "und") {
		return ""
	}
	tag, err := language.Parse(code)
	if err != nil {
		return ""
	}
	return display.English.Languages().Name(tag)
}

func isTextSubtitleCodec(codec string) bool {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "ass", "ssa", "srt", "subrip", "webvtt", "mov_text", "text", "ttml":
		return true
	default:
		return false
	}
}

func probeStreamDisplayTitle(stream ProbeStream) string {
	parts := make([]string, 0, 4)
	switch stream.CodecType {
	case "video":
		if resolution := probeResolutionLabel(stream.Width, stream.Height); resolution != "" {
			parts = append(parts, resolution)
		}
		if videoRange := probeDocVideoRange(stream); videoRange != "" && videoRange != "SDR" {
			parts = append(parts, videoRange)
		}
	case "audio":
		if title := strings.TrimSpace(stream.Tags.Title); title != "" {
			parts = append(parts, title)
		}
		if language := probeDisplayLanguage(stream.Tags.Language); language != "" {
			parts = append(parts, language)
		}
		if codec := strings.TrimSpace(stream.CodecName); codec != "" {
			parts = append(parts, strings.ToUpper(codec))
		}
		if layout := probeChannelLayoutLabel(stream); layout != "" {
			parts = append(parts, layout)
		}
		if stream.Disposition.Default {
			parts = append(parts, "(默认)")
		}
	case "subtitle":
		if title := strings.TrimSpace(stream.Tags.Title); title != "" {
			parts = append(parts, title)
		}
		if language := probeDisplayLanguage(stream.Tags.Language); language != "" {
			parts = append(parts, language)
		}
	}
	if stream.CodecType != "audio" {
		if codec := strings.TrimSpace(stream.CodecName); codec != "" {
			parts = append(parts, strings.ToUpper(codec))
		}
	}
	if stream.Disposition.Forced {
		parts = append(parts, "(强制)")
	}
	return strings.Join(parts, " ")
}

func probeResolutionLabel(width, height int) string {
	switch {
	case width >= 7000 || height >= 4000:
		return "8K"
	case width >= 3800 || height >= 2000:
		return "4K"
	case height >= 1080 || width >= 1900:
		return "1080p"
	case height >= 720 || width >= 1260:
		return "720p"
	case width > 0 && height > 0:
		return fmt.Sprintf("%dx%d", width, height)
	default:
		return ""
	}
}

func probeChannelLayoutLabel(stream ProbeStream) string {
	if layout := strings.TrimSpace(stream.ChannelLayout); layout != "" {
		return layout
	}
	switch stream.Channels {
	case 0:
		return ""
	case 1:
		return "mono"
	case 2:
		return "stereo"
	case 6:
		return "5.1"
	case 8:
		return "7.1"
	default:
		return strconv.Itoa(stream.Channels) + "ch"
	}
}

func probeDocVideoRange(stream ProbeStream) string {
	for _, side := range stream.SideData {
		lower := strings.ToLower(side)
		if strings.Contains(lower, "dovi") || strings.Contains(lower, "dolby vision") {
			return "Dolby Vision"
		}
		if strings.Contains(lower, "hdr10+") || strings.Contains(lower, "smpte2094") {
			return "HDR10+"
		}
	}
	switch strings.ToLower(strings.TrimSpace(stream.ColorTransfer)) {
	case "smpte2084":
		return "HDR10"
	case "arib-std-b67":
		return "HLG"
	case "bt709", "smpte170m", "bt470m", "bt470bg", "iec61966-2-1", "gamma22", "gamma28":
		return "SDR"
	default:
		return ""
	}
}

// probeExtendedVideoType 映射为 Emby 的 ExtendedVideoType 三元组。
func probeExtendedVideoType(videoRange string) (string, string, string) {
	switch videoRange {
	case "HDR10":
		return "Hdr10", "Hdr10", "HDR 10"
	case "HDR10+":
		return "Hdr10Plus", "Hdr10Plus0", "HDR 10+"
	case "Dolby Vision":
		return "DolbyVision", "None", "Dolby Vision"
	case "HLG":
		return "HyperLogGamma", "HyperLogGamma", "Hybrid Log-Gamma"
	default:
		return "", "", ""
	}
}
