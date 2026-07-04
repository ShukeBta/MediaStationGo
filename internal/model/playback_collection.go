package model

import "time"

// PlaybackHistory 记录当前播放位置以支持续播。
type PlaybackHistory struct {
	Base
	UserID     string    `gorm:"index;size:36;not null" json:"user_id"`
	MediaID    string    `gorm:"index;size:128;not null" json:"media_id"`
	PositionMs int64     `json:"position_ms"`
	DurationMs int64     `json:"duration_ms"`
	WatchedAt  time.Time `json:"watched_at"`
	Completed  bool      `json:"completed"`
}

// Favorite 将媒体项标记为给定用户的收藏。
type Favorite struct {
	Base
	UserID  string `gorm:"index;size:36;not null;uniqueIndex:uniq_user_media" json:"user_id"`
	MediaID string `gorm:"index;size:128;not null;uniqueIndex:uniq_user_media" json:"media_id"`
}

// Playlist 是用户策划的、有序的媒体列表。
type Playlist struct {
	Base
	UserID   string `gorm:"index;size:36;not null" json:"user_id"`
	Name     string `gorm:"size:128;not null" json:"name"`
	IsPublic bool   `gorm:"default:false" json:"is_public"`
}

// PlaylistItem 是 Playlist 和 Media 的连接表，带有排序。
type PlaylistItem struct {
	Base
	PlaylistID string `gorm:"index;size:36;not null" json:"playlist_id"`
	MediaID    string `gorm:"index;size:128;not null" json:"media_id"`
	Position   int    `json:"position"`
}

// PlayProfile lets one user define multiple "viewing personas" with
// different content-rating limits, library access, and player defaults.
// The original Vue project sketched this out as a forward-looking
// feature; we materialise it server-side so the React port can fully
// function without dropping the screen.
//
// AllowedLibraryIDs is a JSON array of library UUIDs (empty = all).
type PlayProfile struct {
	Base
	UserID                string     `gorm:"index;size:36;not null" json:"user_id"`
	Name                  string     `gorm:"size:64;not null" json:"name"`
	IsDefault             bool       `gorm:"default:false" json:"is_default"`
	ContentRatingLimit    string     `gorm:"size:16" json:"content_rating_limit,omitempty"`
	AllowAdult            bool       `gorm:"default:false" json:"allow_adult"`
	RequirePIN            bool       `gorm:"default:false" json:"require_pin"`
	PINHash               string     `gorm:"size:128" json:"-"`
	PreferredSubtitleLang string     `gorm:"size:16" json:"preferred_subtitle_lang,omitempty"`
	PreferredAudioLang    string     `gorm:"size:16" json:"preferred_audio_lang,omitempty"`
	AutoplayNext          bool       `gorm:"default:true" json:"autoplay_next"`
	SkipIntro             bool       `gorm:"default:false" json:"skip_intro"`
	AllowedLibraryIDs     string     `gorm:"type:text;default:'[]'" json:"allowed_library_ids"`
	TotalWatchTime        int64      `gorm:"default:0" json:"total_watch_time"`
	LastActiveAt          *time.Time `json:"last_active_at,omitempty"`
}
