package model

// EmbyPlaybackInfoRequest 播放信息请求。
type EmbyPlaybackInfoRequest struct {
	UserId              string             `json:"UserId,omitempty"`
	MaxStreamingBitrate int64              `json:"MaxStreamingBitrate,omitempty"`
	StartTimeTicks      int64              `json:"StartTimeTicks,omitempty"`
	AudioStreamIndex    int                `json:"AudioStreamIndex,omitempty"`
	SubtitleStreamIndex int                `json:"SubtitleStreamIndex,omitempty"`
	MaxAudioChannels    int                `json:"MaxAudioChannels,omitempty"`
	ItemId              string             `json:"ItemId,omitempty"`
	DeviceProfile       *EmbyDeviceProfile `json:"DeviceProfile,omitempty"`
	EnableDirectStream  bool               `json:"EnableDirectStream,omitempty"`
	EnableDirectPlay    bool               `json:"EnableDirectPlay,omitempty"`
	AutoOpenLiveStream  bool               `json:"AutoOpenLiveStream,omitempty"`
}

// EmbyPlaybackInfoResponse 播放信息响应。
type EmbyPlaybackInfoResponse struct {
	MediaSources  []EmbyMediaSource `json:"MediaSources"`
	PlaySessionId string            `json:"PlaySessionId"`
}

// EmbyDeviceProfile 设备配置文件。
type EmbyDeviceProfile struct {
	Name                             string                   `json:"Name,omitempty"`
	MaxStaticBitrate                 int                      `json:"MaxStaticBitrate,omitempty"`
	MaxStreamingBitrate              int                      `json:"MaxStreamingBitrate,omitempty"`
	MusicStreamingTranscodingBitrate int                      `json:"MusicStreamingTranscodingBitrate,omitempty"`
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
	Container                 string `json:"Container,omitempty"`
	Type                      string `json:"Type,omitempty"`
	VideoCodec                string `json:"VideoCodec,omitempty"`
	AudioCodec                string `json:"AudioCodec,omitempty"`
	Protocol                  string `json:"Protocol,omitempty"`
	EstimateContentLength     bool   `json:"EstimateContentLength,omitempty"`
	EnableMpegtsM2TsMode      bool   `json:"EnableMpegtsM2TsMode,omitempty"`
	TranscodeSeekInfo         string `json:"TranscodeSeekInfo,omitempty"`
	Context                   string `json:"Context,omitempty"`
	EnableSubtitlesInManifest bool   `json:"EnableSubtitlesInManifest,omitempty"`
	MaxAudioChannels          int    `json:"MaxAudioChannels,omitempty"`
	MinSegments               int    `json:"MinSegments,omitempty"`
	SegmentLength             int    `json:"SegmentLength,omitempty"`
	BreakOnNonKeyFrames       bool   `json:"BreakOnNonKeyFrames,omitempty"`
}

// EmbyContainerProfile 容器配置。
type EmbyContainerProfile struct {
	Type       string   `json:"Type,omitempty"`
	Conditions []string `json:"Conditions,omitempty"`
	Container  string   `json:"Container,omitempty"`
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
	Condition  string `json:"Condition,omitempty"`
	Property   string `json:"Property,omitempty"`
	Value      string `json:"Value,omitempty"`
	IsRequired bool   `json:"IsRequired,omitempty"`
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
	CanSeek             bool     `json:"CanSeek"`
	ItemId              string   `json:"ItemId"`
	MediaSourceId       string   `json:"MediaSourceId,omitempty"`
	PositionTicks       int64    `json:"PositionTicks"`
	RunTimeTicks        int64    `json:"RunTimeTicks,omitempty"`
	IsPaused            bool     `json:"IsPaused"`
	IsMuted             bool     `json:"IsMuted"`
	VolumeLevel         int      `json:"VolumeLevel,omitempty"`
	PlayMethod          string   `json:"PlayMethod,omitempty"`
	PlaySessionId       string   `json:"PlaySessionId,omitempty"`
	LiveStreamId        string   `json:"LiveStreamId,omitempty"`
	QueueableMediaTypes []string `json:"QueueableMediaTypes,omitempty"`
}

// EmbyStopPlaybackRequest 停止播放上报。
type EmbyStopPlaybackRequest struct {
	ItemId        string `json:"ItemId"`
	MediaSourceId string `json:"MediaSourceId,omitempty"`
	PositionTicks int64  `json:"PositionTicks"`
	RunTimeTicks  int64  `json:"RunTimeTicks,omitempty"`
	PlaySessionId string `json:"PlaySessionId,omitempty"`
	LiveStreamId  string `json:"LiveStreamId,omitempty"`
}

// EmbyUserDataRequest 用户数据更新。
type EmbyUserDataRequest struct {
	PlaybackPositionTicks int64   `json:"PlaybackPositionTicks,omitempty"`
	PlayCount             int     `json:"PlayCount,omitempty"`
	IsFavorite            bool    `json:"IsFavorite,omitempty"`
	Played                bool    `json:"Played,omitempty"`
	PlayedPercentage      float64 `json:"PlayedPercentage,omitempty"`
}

// EmbyActiveEncodingRequest 活跃编码请求（客户端报告转码进度）。
type EmbyActiveEncodingRequest struct {
	PlaySessionId string `json:"PlaySessionId"`
	When          string `json:"When"`
	PositionTicks int64  `json:"PositionTicks,omitempty"`
	IsPaused      bool   `json:"IsPaused,omitempty"`
	IsUserPaused  bool   `json:"IsUserPaused,omitempty"`
}
