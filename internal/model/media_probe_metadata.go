package model

import "time"

// MediaProbeMetadata 保存单个可播放媒体的完整 ffprobe 轨道文档(白名单字段)。
// MediaID 同时作为主键,每个 Media 最多一份;媒体删除或路径变化时由数据库
// 触发器清理(见 database.ensureMediaProbeMetadataCleanup)。
type MediaProbeMetadata struct {
	MediaID       string    `gorm:"primaryKey;size:36;not null" json:"media_id"`
	ProbeJSON     string    `gorm:"type:text;not null" json:"-"`
	SchemaVersion int       `gorm:"not null" json:"schema_version"`
	SizeBytes     int64     `gorm:"not null;default:0" json:"size_bytes"`
	SourceKey     string    `gorm:"size:64;not null;default:''" json:"-"`
	ProbedAt      time.Time `gorm:"not null" json:"probed_at"`
}

func (MediaProbeMetadata) TableName() string {
	return "media_probe_metadata"
}

// MediaTrack 是面向详情页/Emby 的安全轨道投影,不包含路径、URL 或任意探测标签。
type MediaTrack struct {
	Index             int     `json:"index"`
	Type              string  `json:"type"`
	Codec             string  `json:"codec,omitempty"`
	Profile           string  `json:"profile,omitempty"`
	Level             int     `json:"level,omitempty"`
	TimeBase          string  `json:"time_base,omitempty"`
	Language          string  `json:"language,omitempty"`
	DisplayLanguage   string  `json:"display_language,omitempty"`
	Title             string  `json:"title,omitempty"`
	DisplayTitle      string  `json:"display_title,omitempty"`
	BitRate           int64   `json:"bit_rate,omitempty"`
	IsDefault         bool    `json:"is_default"`
	IsForced          bool    `json:"is_forced"`
	IsHearingImpaired bool    `json:"is_hearing_impaired,omitempty"`
	IsVisualImpaired  bool    `json:"is_visual_impaired,omitempty"`
	Width             int     `json:"width,omitempty"`
	Height            int     `json:"height,omitempty"`
	AspectRatio       string  `json:"aspect_ratio,omitempty"`
	PixelFormat       string  `json:"pixel_format,omitempty"`
	BitDepth          int     `json:"bit_depth,omitempty"`
	ColorRange        string  `json:"color_range,omitempty"`
	ColorSpace        string  `json:"color_space,omitempty"`
	ColorTransfer     string  `json:"color_transfer,omitempty"`
	ColorPrimaries    string  `json:"color_primaries,omitempty"`
	VideoRange        string  `json:"video_range,omitempty"`
	AverageFrameRate  float64 `json:"average_frame_rate,omitempty"`
	RealFrameRate     float64 `json:"real_frame_rate,omitempty"`
	Channels          int     `json:"channels,omitempty"`
	SampleRate        int     `json:"sample_rate,omitempty"`
	ChannelLayout     string  `json:"channel_layout,omitempty"`
	SampleFormat      string  `json:"sample_format,omitempty"`
	BitsPerSample     int     `json:"bits_per_sample,omitempty"`
	IsTextSubtitle    bool    `json:"is_text_subtitle,omitempty"`
}
