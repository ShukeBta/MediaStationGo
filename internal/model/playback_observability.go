package model

import "time"

// PlaybackEvent is one qualified playback session, independent of resume history.
type PlaybackEvent struct {
	Base
	EventKey   string    `gorm:"size:64;not null;uniqueIndex" json:"-"`
	UserID     string    `gorm:"size:36;index" json:"user_id"`
	MediaID    string    `gorm:"size:128;index" json:"media_id"`
	LibraryID  string    `gorm:"size:36;index" json:"library_id"`
	PlayedAt   time.Time `gorm:"not null;index" json:"played_at"`
	MediaType  string    `gorm:"size:16;index" json:"media_type"`
	WorkKey    string    `gorm:"size:128;index" json:"work_key"`
	Title      string    `gorm:"type:text" json:"title"`
	PosterURL  string    `gorm:"type:text" json:"poster_url,omitempty"`
	SeasonNum  int       `json:"season_num,omitempty"`
	EpisodeNum int       `json:"episode_num,omitempty"`
	Client     string    `gorm:"size:128" json:"client,omitempty"`
}

// PlayerRequestLog deliberately excludes arbitrary headers, URLs and bodies:
// authentication credentials and signed media addresses must never be persisted.
type PlayerRequestLog struct {
	Base
	RequestedAt time.Time `gorm:"not null;index" json:"requested_at"`
	Method      string    `gorm:"size:16;index" json:"method"`
	Route       string    `gorm:"size:255;index" json:"route"`
	Status      int       `gorm:"index" json:"status"`
	DurationMS  int64     `json:"duration_ms"`
	IP          string    `gorm:"size:64" json:"ip"`
	UserID      string    `gorm:"size:36;index" json:"user_id,omitempty"`
	Query       string    `gorm:"type:text" json:"query"`
}
