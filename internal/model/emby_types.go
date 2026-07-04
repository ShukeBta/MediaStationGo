// Package model — Emby API 兼容层请求/响应类型。
package model

import "time"

// EmbyAuthRequest Emby 认证请求（用户名+密码）。
type EmbyAuthRequest struct {
	Username string `json:"Username"`
	Password string `json:"Password"`
}

// EmbyAuthResponse Emby 认证响应。
type EmbyAuthResponse struct {
	User        EmbyUser `json:"User"`
	AccessToken string   `json:"AccessToken"`
	ServerID    string   `json:"ServerId"`
}

// EmbyApiKeyAuthRequest API Key 认证请求。
type EmbyApiKeyAuthRequest struct {
	ApiKey string `json:"ApiKey"`
}

// EmbyUser Emby 用户信息。
type EmbyUser struct {
	Id               string                 `json:"Id"`
	Name             string                 `json:"Name"`
	ServerId         string                 `json:"ServerId"`
	HasPassword      bool                   `json:"HasPassword"`
	PrimaryImageTag  string                 `json:"PrimaryImageTag,omitempty"`
	Configuration    *EmbyUserConfiguration `json:"Configuration,omitempty"`
	LastActivityDate *time.Time             `json:"LastActivityDate,omitempty"`
	LastLoginDate    *time.Time             `json:"LastLoginDate,omitempty"`
}

// EmbyUserConfiguration 用户配置。
type EmbyUserConfiguration struct {
	AudioLanguagePreference    string `json:"AudioLanguagePreference"`
	SubtitleLanguagePreference string `json:"SubtitleLanguagePreference"`
	EnableAutoPlay             bool   `json:"EnableAutoPlay"`
	EnableNextEpisodeAutoPlay  bool   `json:"EnableNextEpisodeAutoPlay"`
}

// EmbySystemInfo 系统信息。
type EmbySystemInfo struct {
	Id                    string `json:"Id"`
	ServerName            string `json:"ServerName"`
	Version               string `json:"Version"`
	ProductName           string `json:"ProductName"`
	OperatingSystem       string `json:"OperatingSystem"`
	Architecture          string `json:"Architecture"`
	LocalAddress          string `json:"LocalAddress"`
	WanAddress            string `json:"WanAddress,omitempty"`
	HasPendingRestart     bool   `json:"HasPendingRestart"`
	IsShuttingDown        bool   `json:"IsShuttingDown"`
	SupportsLibraryScan   bool   `json:"SupportsLibraryScan"`
	SupportsHttps         bool   `json:"SupportsHttps"`
	SupportsAutoDiscovery bool   `json:"SupportsAutoDiscovery"`
	WebSocketPortNumber   int    `json:"WebSocketPortNumber"`
	TranscodingTempPath   string `json:"TranscodingTempPath,omitempty"`
	CanSelfUpdate         bool   `json:"CanSelfUpdate"`
	CanLaunchWebBrowser   bool   `json:"CanLaunchWebBrowser"`
	CanRestart            bool   `json:"CanRestart"`
	CodecCount            int    `json:"CodecCount"`
}

// EmbyLogEntry 日志条目。
type EmbyLogEntry struct {
	Id          string    `json:"Id"`
	DateCreated time.Time `json:"DateCreated"`
	Level       string    `json:"Level"`
	Message     string    `json:"Message"`
}

// EmbyServerConfiguration 服务器配置。
type EmbyServerConfiguration struct {
	Name                 string `json:"Name"`
	ServerName           string `json:"ServerName"`
	EnableUPnP           bool   `json:"EnableUPnP"`
	PublicPort           int    `json:"PublicPort"`
	EnableHttps          bool   `json:"EnableHttps"`
	HttpServerPortNumber int    `json:"HttpServerPortNumber"`
	HttpsPortNumber      int    `json:"HttpsPortNumber"`
	EnableRemoteAccess   bool   `json:"EnableRemoteAccess"`
}

// EmbySession 会话信息。
type EmbySession struct {
	Id                    string                 `json:"Id"`
	Client                string                 `json:"Client"`
	ClientVersion         string                 `json:"ClientVersion"`
	DeviceId              string                 `json:"DeviceId"`
	DeviceName            string                 `json:"DeviceName"`
	UserName              string                 `json:"UserName,omitempty"`
	UserId                string                 `json:"UserId,omitempty"`
	LastActivityDate      *time.Time             `json:"LastActivityDate,omitempty"`
	RemoteEndPoint        string                 `json:"RemoteEndPoint,omitempty"`
	NowPlayingItem        *EmbyItem              `json:"NowPlayingItem,omitempty"`
	PlayState             *EmbyPlaybackState     `json:"PlayState,omitempty"`
	Capabilities          EmbyClientCapabilities `json:"Capabilities,omitempty"`
	SupportsRemoteControl bool                   `json:"SupportsRemoteControl"`
	AdditionalUsers       []EmbySessionUserInfo  `json:"AdditionalUsers,omitempty"`
}

// EmbySessionUserInfo 会话中的附加用户。
type EmbySessionUserInfo struct {
	UserId   string `json:"UserId"`
	UserName string `json:"UserName"`
}

// EmbyPlaybackState 播放状态。
type EmbyPlaybackState struct {
	PositionTicks int64  `json:"PositionTicks"`
	VolumeLevel   int    `json:"VolumeLevel"`
	IsMuted       bool   `json:"IsMuted"`
	IsPaused      bool   `json:"IsPaused"`
	PlayMethod    string `json:"PlayMethod,omitempty"`
	CanSeek       bool   `json:"CanSeek"`
}

// EmbyClientCapabilities 客户端能力描述。
type EmbyClientCapabilities struct {
	PlayableMediaTypes   []string `json:"PlayableMediaTypes"`
	SupportedCommands    []string `json:"SupportedCommands"`
	SupportsMediaControl bool     `json:"SupportsMediaControl"`
	SupportsSync         bool     `json:"SupportsSync"`
}
