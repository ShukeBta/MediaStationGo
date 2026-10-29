package model

import (
	"fmt"
	"strconv"
	"strings"
)

// FlexInt 兼容把数值写成字符串的客户端（如 Hills："MinSegments":"1"）。
type FlexInt int

func (f *FlexInt) UnmarshalJSON(b []byte) error {
	v, err := parseFlexNumber(string(b), 32)
	if err != nil {
		return err
	}
	*f = FlexInt(v)
	return nil
}

// FlexInt64 同 FlexInt，用于 Ticks 等大数值。
type FlexInt64 int64

func (f *FlexInt64) UnmarshalJSON(b []byte) error {
	v, err := parseFlexNumber(string(b), 64)
	if err != nil {
		return err
	}
	*f = FlexInt64(v)
	return nil
}

// FlexBool 兼容把布尔写成字符串的客户端（如 Hills："BreakOnNonKeyFrames":"false"）。
type FlexBool bool

func (f *FlexBool) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "null" {
		*f = false
		return nil
	}
	if len(s) >= 2 && s[0] == '"' {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	switch strings.ToLower(s) {
	case "", "false", "0", "f", "no":
		*f = false
	case "true", "1", "t", "yes":
		*f = true
	default:
		return fmt.Errorf("invalid boolean value %q", string(b))
	}
	return nil
}

func parseFlexNumber(raw string, bitSize int) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "null" {
		return 0, nil
	}
	if len(s) >= 2 && s[0] == '"' {
		s = strings.TrimSpace(s[1 : len(s)-1])
	}
	if s == "" {
		return 0, nil
	}
	v, err := strconv.ParseInt(s, 10, bitSize)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value %q", raw)
	}
	return v, nil
}

// EmbyPlaybackInfoRequest 播放信息请求。
type EmbyPlaybackInfoRequest struct {
	UserId              string             `json:"UserId,omitempty"`
	MediaSourceId       string             `json:"MediaSourceId,omitempty"`
	IsPlayback          FlexBool           `json:"IsPlayback,omitempty"`
	MaxStreamingBitrate FlexInt64          `json:"MaxStreamingBitrate,omitempty"`
	StartTimeTicks      FlexInt64          `json:"StartTimeTicks,omitempty"`
	AudioStreamIndex    FlexInt            `json:"AudioStreamIndex,omitempty"`
	SubtitleStreamIndex FlexInt            `json:"SubtitleStreamIndex,omitempty"`
	MaxAudioChannels    int                `json:"MaxAudioChannels,omitempty"`
	ItemId              string             `json:"ItemId,omitempty"`
	DeviceProfile       *EmbyDeviceProfile `json:"DeviceProfile,omitempty"`
	EnableDirectStream  FlexBool           `json:"EnableDirectStream,omitempty"`
	EnableDirectPlay    FlexBool           `json:"EnableDirectPlay,omitempty"`
	AutoOpenLiveStream  FlexBool           `json:"AutoOpenLiveStream,omitempty"`
}

// EmbyPlaybackInfoResponse 播放信息响应。
type EmbyPlaybackInfoResponse struct {
	MediaSources  []EmbyMediaSource `json:"MediaSources"`
	PlaySessionId string            `json:"PlaySessionId"`
}

// EmbyDeviceProfile 设备配置文件。
type EmbyDeviceProfile struct {
	Name                             string                   `json:"Name,omitempty"`
	MaxStaticBitrate                 FlexInt                  `json:"MaxStaticBitrate,omitempty"`
	MaxStreamingBitrate              FlexInt                  `json:"MaxStreamingBitrate,omitempty"`
	MusicStreamingTranscodingBitrate FlexInt                  `json:"MusicStreamingTranscodingBitrate,omitempty"`
	DirectPlayProfiles               []EmbyDirectPlayProfile  `json:"DirectPlayProfiles,omitempty"`
	TranscodingProfiles              []EmbyTranscodingProfile `json:"TranscodingProfiles,omitempty"`
	ContainerProfiles                []EmbyContainerProfile   `json:"ContainerProfiles,omitempty"`
	CodecProfiles                    []EmbyCodecProfile       `json:"CodecProfiles,omitempty"`
	SubtitleProfiles                 []EmbySubtitleProfile    `json:"SubtitleProfiles,omitempty"`
}

// EmbyDirectPlayProfile 直接播放配置。
type EmbyDirectPlayProfile struct {
	Container  string `json:"Container,omitempty"`
	AudioCodec string `json:"AudioCodec,omitempty"`
	VideoCodec string `json:"VideoCodec,omitempty"`
	Type       string `json:"Type,omitempty"`
}

// EmbyTranscodingProfile 转码配置。
type EmbyTranscodingProfile struct {
	Container                 string   `json:"Container,omitempty"`
	Type                      string   `json:"Type,omitempty"`
	VideoCodec                string   `json:"VideoCodec,omitempty"`
	AudioCodec                string   `json:"AudioCodec,omitempty"`
	Protocol                  string   `json:"Protocol,omitempty"`
	EstimateContentLength     FlexBool `json:"EstimateContentLength,omitempty"`
	EnableMpegtsM2TsMode      FlexBool `json:"EnableMpegtsM2TsMode,omitempty"`
	TranscodeSeekInfo         string   `json:"TranscodeSeekInfo,omitempty"`
	Context                   string   `json:"Context,omitempty"`
	EnableSubtitlesInManifest FlexBool `json:"EnableSubtitlesInManifest,omitempty"`
	MaxAudioChannels          string   `json:"MaxAudioChannels,omitempty"`
	MinSegments               FlexInt  `json:"MinSegments,omitempty"`
	SegmentLength             FlexInt  `json:"SegmentLength,omitempty"`
	BreakOnNonKeyFrames       FlexBool `json:"BreakOnNonKeyFrames,omitempty"`
}

// EmbyContainerProfile 容器配置。
type EmbyContainerProfile struct {
	Type       string                 `json:"Type,omitempty"`
	Conditions []EmbyProfileCondition `json:"Conditions,omitempty"`
	Container  string                 `json:"Container,omitempty"`
}

// EmbyCodecProfile 编解码器配置。
type EmbyCodecProfile struct {
	Type       string                 `json:"Type,omitempty"`
	Conditions []EmbyProfileCondition `json:"Conditions,omitempty"`
	Codec      string                 `json:"Codec,omitempty"`
	Container  string                 `json:"Container,omitempty"`
}

// EmbyProfileCondition 配置条件。
type EmbyProfileCondition struct {
	Condition  string   `json:"Condition,omitempty"`
	Property   string   `json:"Property,omitempty"`
	Value      string   `json:"Value,omitempty"`
	IsRequired FlexBool `json:"IsRequired,omitempty"`
}

// EmbySubtitleProfile 字幕配置。
type EmbySubtitleProfile struct {
	Format    string `json:"Format,omitempty"`
	Method    string `json:"Method,omitempty"`
	DidlMode  string `json:"DidlMode,omitempty"`
	Language  string `json:"Language,omitempty"`
	Container string `json:"Container,omitempty"`
}

// EmbyPlaybackProgressRequest 播放进度上报。
type EmbyPlaybackProgressRequest struct {
	CanSeek             bool      `json:"CanSeek"`
	ItemId              string    `json:"ItemId"`
	MediaSourceId       string    `json:"MediaSourceId,omitempty"`
	PositionTicks       FlexInt64 `json:"PositionTicks"`
	RunTimeTicks        FlexInt64 `json:"RunTimeTicks,omitempty"`
	IsPaused            bool      `json:"IsPaused"`
	IsMuted             bool      `json:"IsMuted"`
	VolumeLevel         FlexInt   `json:"VolumeLevel,omitempty"`
	PlayMethod          string    `json:"PlayMethod,omitempty"`
	PlaySessionId       string    `json:"PlaySessionId,omitempty"`
	LiveStreamId        string    `json:"LiveStreamId,omitempty"`
	QueueableMediaTypes []string  `json:"QueueableMediaTypes,omitempty"`
}

// EmbyStopPlaybackRequest 停止播放上报。
type EmbyStopPlaybackRequest struct {
	ItemId        string    `json:"ItemId"`
	MediaSourceId string    `json:"MediaSourceId,omitempty"`
	PositionTicks FlexInt64 `json:"PositionTicks"`
	RunTimeTicks  FlexInt64 `json:"RunTimeTicks,omitempty"`
	PlaySessionId string    `json:"PlaySessionId,omitempty"`
	LiveStreamId  string    `json:"LiveStreamId,omitempty"`
}

// EmbyUserDataRequest 用户数据更新。
type EmbyUserDataRequest struct {
	PlaybackPositionTicks FlexInt64 `json:"PlaybackPositionTicks,omitempty"`
	PlayCount             FlexInt   `json:"PlayCount,omitempty"`
	IsFavorite            bool      `json:"IsFavorite,omitempty"`
	Played                bool      `json:"Played,omitempty"`
	PlayedPercentage      float64   `json:"PlayedPercentage,omitempty"`
}

// EmbyActiveEncodingRequest 活跃编码请求（客户端报告转码进度）。
type EmbyActiveEncodingRequest struct {
	PlaySessionId string    `json:"PlaySessionId"`
	When          string    `json:"When"`
	PositionTicks FlexInt64 `json:"PositionTicks,omitempty"`
	IsPaused      bool      `json:"IsPaused,omitempty"`
	IsUserPaused  bool      `json:"IsUserPaused,omitempty"`
}
