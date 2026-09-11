package model

// MediaDanmaku 持久化「媒体 ↔ 弹幕库(episodeId)」的关联。

// 社区实现（dd-danmaku）把匹配结果存在浏览器 localStorage 里，换设备或换客户端就丢；
// 这里放在服务端，三端共享同一份关联，并且记录官方 /match 下发的 shift 与用户自定义偏移。
type MediaDanmaku struct {
	Base
	MediaID    string `gorm:"uniqueIndex;size:128;not null" json:"media_id"`
	Provider   string `gorm:"size:32;not null" json:"provider"`
	EpisodeID  string `gorm:"size:32;not null" json:"episode_id"`
	AnimeTitle string `gorm:"size:255" json:"anime_title,omitempty"`
	// EpisodeTitle 与 MatchMode 用于在 UI 上解释「这条弹幕是怎么匹配上的」。
	EpisodeTitle string  `gorm:"size:255" json:"episode_title,omitempty"`
	MatchMode    string  `gorm:"size:16" json:"match_mode,omitempty"`
	Confidence   float64 `json:"confidence,omitempty"`
	// ProviderShiftSeconds 是弹弹play /api/v2/match 返回的 shift，必须叠加到弹幕时间上。
	ProviderShiftSeconds float64 `gorm:"not null;default:0" json:"provider_shift_seconds"`
	// OffsetSeconds 是用户手动的时间轴修正，叠加在 provider shift 之后。
	OffsetSeconds float64 `gorm:"not null;default:0" json:"offset_seconds"`
	// Status 取值 matched / unmatched / failed；unmatched 也要落库，避免每次播放都重复回源。
	Status string `gorm:"size:16;not null;default:matched" json:"status"`
	// Attempts 保存匹配过程（JSON 文本），未匹配时用于向用户解释原因。
	Attempts string `gorm:"type:text" json:"-"`
}
