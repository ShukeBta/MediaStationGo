// Package model 定义 GORM 数据模型和自动迁移使用的注册表。
// 每个子系统在 MediaStationGo 中拥有一个表切片；AllModels 返回联合以供 db.AutoMigrate 使用。
package model

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Base 嵌入每个域实体共享的字段:
//
//   - ID:         UUID v4 字符串主键
//   - CreatedAt / UpdatedAt: 由 GORM 管理
//   - DeletedAt:  软删除（查询自动过滤）
type Base struct {
	ID        string         `gorm:"primaryKey;type:varchar(36)" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// BeforeCreate 如果调用者未提供则生成 UUID。
func (b *Base) BeforeCreate(_ *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	return nil
}

// User 是本地账户。第一个注册的管理员（或种子管理员）获得 "admin" 角色；
// 其他所有用户默认为 "user"。
type User struct {
	Base
	Username           string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash       string     `gorm:"size:128;not null" json:"-"`
	Role               string     `gorm:"size:16;not null;default:user" json:"role"`
	Tier               string     `gorm:"size:16;default:free" json:"tier"` // free / plus
	Nickname           string     `gorm:"size:128" json:"nickname,omitempty"`
	Email              string     `gorm:"size:128" json:"email,omitempty"`
	AvatarURL          string     `gorm:"size:255" json:"avatar_url,omitempty"`
	HideAdult          bool       `gorm:"default:true" json:"hide_adult"`
	ForcePasswordReset bool       `gorm:"default:false" json:"force_password_reset"`
	IsActive           bool       `gorm:"default:true" json:"is_active"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	// ExpiredAt is the account expiry time. Nil means the account never
	// expires. When set and in the past, the account is treated as expired
	// (login blocked) until an admin or a redemption code renews it.
	ExpiredAt *time.Time `json:"expired_at,omitempty"`
	// ShareWarnings counts anti-account-sharing warnings (too many concurrent
	// playbacks / logged-in clients / device-fingerprint mismatches). Once it
	// exceeds the configured threshold a re-offence deletes the account.
	ShareWarnings   int        `gorm:"default:0" json:"share_warnings"`
	LastShareWarnAt *time.Time `json:"last_share_warn_at,omitempty"`
	IsDefaultAdmin  bool       `gorm:"-" json:"is_default_admin,omitempty"`
	IsProtected     bool       `gorm:"-" json:"is_protected,omitempty"`
}

// Library 表示用户定义的媒体根目录。
type Library struct {
	Base
	Name    string `gorm:"size:128;not null" json:"name"`
	Path    string `gorm:"size:1024;not null" json:"path"`
	Type    string `gorm:"size:16;not null;default:movie" json:"type"` // movie / tv / anime / music
	Enabled bool   `gorm:"default:true" json:"enabled"`
}

// Media 是单个可播放项。剧集链接到 SeriesID；电影 SeriesID == ""。
type Media struct {
	Base
	LibraryID    string  `gorm:"index;size:36" json:"library_id"`
	SeriesID     string  `gorm:"index;size:36" json:"series_id,omitempty"`
	Title        string  `gorm:"size:255;not null" json:"title"`
	OriginalName string  `gorm:"size:255" json:"original_name,omitempty"`
	Path         string  `gorm:"uniqueIndex;size:1024;not null" json:"path"`
	SizeBytes    int64   `json:"size_bytes"`
	DurationSec  int     `json:"duration_sec"`
	Width        int     `json:"width"`
	Height       int     `json:"height"`
	VideoCodec   string  `gorm:"size:32" json:"video_codec,omitempty"`
	AudioCodec   string  `gorm:"size:32" json:"audio_codec,omitempty"`
	Container    string  `gorm:"size:16" json:"container,omitempty"`
	PosterURL    string  `gorm:"size:1024" json:"poster_url,omitempty"`
	BackdropURL  string  `gorm:"size:1024" json:"backdrop_url,omitempty"`
	Overview     string  `gorm:"type:text" json:"overview,omitempty"`
	Rating       float32 `json:"rating"`
	Year         int     `json:"year"`
	SeasonNum    int     `json:"season_num"`
	EpisodeNum   int     `json:"episode_num"`
	ScrapeStatus string  `gorm:"size:16;default:pending" json:"scrape_status"`
	TMDbID       int     `json:"tmdb_id"`
	BangumiID    int     `json:"bangumi_id"`
	Languages    string  `gorm:"size:64"  json:"languages,omitempty"` // 逗号分隔的 ISO 639-1 代码，如 "zh,en"
	Countries    string  `gorm:"size:128" json:"countries,omitempty"` // 逗号分隔的 ISO 3166-1，如 "CN,US"
	Genres       string  `gorm:"size:255" json:"genres,omitempty"`    // 逗号分隔的类型名，如 "Action,Animation"
	NSFW         bool    `gorm:"default:false" json:"nsfw"`

	// STRMURL is the indirection target for .strm files: when present the
	// stream handler redirects to it instead of opening the local file.
	// Used to expose WebDAV / Alist / S3 / HTTP direct links as media items.
	STRMURL string `gorm:"size:2048" json:"strm_url,omitempty"`

	// FileHash is a sparse-sample MD5 used for duplicate detection.
	// Computed on-demand by the duplicate finder; format: "<hex>-<size>".
	FileHash string `gorm:"index;size:64" json:"file_hash,omitempty"`

	// FileID is a "device:inode" identity for the underlying file. Hardlinks
	// to the same data share a FileID, letting the scanner skip re-importing a
	// seeding source kept by keep_seeding and its organized hardlink as two
	// separate items (avoids duplicate rows + double-counted storage).
	FileID string `gorm:"index;size:64" json:"file_id,omitempty"`

	// IsDuplicate flags this media as a duplicate of another media row.
	IsDuplicate bool   `gorm:"default:false" json:"is_duplicate"`
	DuplicateOf string `gorm:"size:36" json:"duplicate_of,omitempty"`
}

// APIConfig stores third-party data-source configuration. The api_key
// column is encrypted with AES-GCM (see internal/service/crypto.go) so an
// SQLite leak does not expose third-party credentials.
//
// Provider values mirror the original Python project:
//
//	tmdb        — themoviedb.org
//	bangumi     — bgm.tv
//	thetvdb     — thetvdb.com
//	fanart      — fanart.tv
//	douban      — douban.com (cookie)
//	openai      — OpenAI / DeepSeek / Qwen / Ollama (compatible)
type APIConfig struct {
	Base
	Provider    string `gorm:"uniqueIndex;size:32;not null" json:"provider"`
	APIKey      string `gorm:"type:text" json:"-"` // ciphertext (never serialised)
	BaseURL     string `gorm:"size:512" json:"base_url,omitempty"`
	Extra       string `gorm:"type:text" json:"extra,omitempty"` // free-form JSON
	Enabled     bool   `gorm:"default:true" json:"enabled"`
	Description string `gorm:"size:255" json:"description,omitempty"`
}

// Series 将属于同一节目的剧集分组。
type Series struct {
	Base
	LibraryID   string  `gorm:"index;size:36" json:"library_id"`
	Title       string  `gorm:"size:255;not null" json:"title"`
	PosterURL   string  `gorm:"size:1024" json:"poster_url,omitempty"`
	BackdropURL string  `gorm:"size:1024" json:"backdrop_url,omitempty"`
	Overview    string  `gorm:"type:text" json:"overview,omitempty"`
	Rating      float32 `json:"rating"`
	Year        int     `json:"year"`
	TMDbID      int     `json:"tmdb_id"`
	BangumiID   int     `json:"bangumi_id"`
}

// PlaybackHistory 记录当前播放位置以支持续播。
type PlaybackHistory struct {
	Base
	UserID     string    `gorm:"index;size:36;not null" json:"user_id"`
	MediaID    string    `gorm:"index;size:36;not null" json:"media_id"`
	PositionMs int64     `json:"position_ms"`
	DurationMs int64     `json:"duration_ms"`
	WatchedAt  time.Time `json:"watched_at"`
	Completed  bool      `json:"completed"`
}

// Favorite 将媒体项标记为给定用户的收藏。
type Favorite struct {
	Base
	UserID  string `gorm:"index;size:36;not null;uniqueIndex:uniq_user_media" json:"user_id"`
	MediaID string `gorm:"index;size:36;not null;uniqueIndex:uniq_user_media" json:"media_id"`
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
	MediaID    string `gorm:"index;size:36;not null" json:"media_id"`
	Position   int    `json:"position"`
}

// DownloadTask 是待处理（或已完成）的 torrent / HTTP 下载。
type DownloadTask struct {
	Base
	UserID      string  `gorm:"index;size:36" json:"user_id"`
	Source      string  `gorm:"size:32;not null" json:"source"` // qbittorrent / transmission / http
	URL         string  `gorm:"size:2048;not null" json:"-"`
	Title       string  `gorm:"size:512" json:"title,omitempty"`
	PosterURL   string  `gorm:"size:2048" json:"poster_url,omitempty"`
	BackdropURL string  `gorm:"size:2048" json:"backdrop_url,omitempty"`
	Overview    string  `gorm:"type:text" json:"overview,omitempty"`
	SavePath    string  `gorm:"size:1024" json:"save_path"`
	Status      string  `gorm:"size:32;default:queued" json:"status"`
	Progress    float32 `json:"progress"`
}

// Subscription 是自动化规则，轮询 RSS 源并将匹配种子排队到配置的下载客户端。
type Subscription struct {
	Base
	UserID        string     `gorm:"index;size:36" json:"user_id"`
	Name          string     `gorm:"size:128;not null" json:"name"`
	FeedURL       string     `gorm:"size:2048;not null" json:"feed_url"`
	Filter        string     `gorm:"size:512" json:"filter"`
	MediaType     string     `gorm:"size:16" json:"media_type,omitempty"`
	MediaCategory string     `gorm:"size:128" json:"media_category,omitempty"`
	SavePath      string     `gorm:"size:1024" json:"save_path,omitempty"`
	SearchMode    string     `gorm:"size:16;default:keyword" json:"search_mode,omitempty"` // keyword / imdb
	IMDBID        string     `gorm:"size:32" json:"imdb_id,omitempty"`
	Source        string     `gorm:"size:32" json:"source,omitempty"`
	PosterURL     string     `gorm:"size:2048" json:"poster_url,omitempty"`
	BackdropURL   string     `gorm:"size:2048" json:"backdrop_url,omitempty"`
	Overview      string     `gorm:"type:text" json:"overview,omitempty"`
	Resolution    string     `gorm:"size:32" json:"resolution,omitempty"`      // 2160p / 1080p / 720p / best
	Quality       string     `gorm:"size:64" json:"quality,omitempty"`         // remux / bluray / web-dl / hdtv
	Effects       string     `gorm:"size:128" json:"effects,omitempty"`        // hdr,dolby-vision,atmos
	ReleaseGroups string     `gorm:"size:255" json:"release_groups,omitempty"` // comma separated
	ExcludeWords  string     `gorm:"size:255" json:"exclude_words,omitempty"`  // comma separated
	WashEnabled   bool       `gorm:"default:false" json:"wash_enabled"`
	WashPriority  string     `gorm:"size:32" json:"wash_priority,omitempty"` // balanced / resolution / quality / effects / seeders
	TotalEpisodes int        `gorm:"default:0" json:"total_episodes,omitempty"`
	Priority      int        `gorm:"default:50" json:"priority,omitempty"` // lower is earlier when schedulers sort later
	Enabled       bool       `gorm:"default:true" json:"enabled"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`

	DownloadedEpisodes int   `gorm:"-" json:"downloaded_episodes,omitempty"`
	LocalMediaCount    int   `gorm:"-" json:"local_media_count,omitempty"`
	MissingEpisodes    []int `gorm:"-" json:"missing_episodes,omitempty"`
	InLibrary          bool  `gorm:"-" json:"in_library"`
}

// Setting 是单个键/值系统级偏好（供管理 UI 使用）。
type Setting struct {
	Key       string    `gorm:"primaryKey;size:128" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AccessLog 是结构化审计跟踪条目。存储在 SQLite 中供管理活动面板使用。
type AccessLog struct {
	Base
	UserID string `gorm:"index;size:36" json:"user_id"`
	Action string `gorm:"size:64;not null" json:"action"`
	Target string `gorm:"size:255" json:"target"`
	IP     string `gorm:"size:64" json:"ip"`
	Detail string `gorm:"type:text" json:"detail"`
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

// StorageConfig holds the connection settings for one external storage
// backend (Alist / S3 / WebDAV). Type column makes the row poly-typed
// — Config is a JSON blob whose shape is determined by Type.
//
//	alist  → {server, token}
//	s3     → {endpoint, region, bucket, access_key, secret_key, force_path_style}
//	webdav → {url, username, password}
type StorageConfig struct {
	Base
	Type      string `gorm:"uniqueIndex;size:16;not null" json:"type"`
	Config    string `gorm:"type:text;not null" json:"-"` // ciphertext
	Enabled   bool   `gorm:"default:true" json:"enabled"`
	LastError string `gorm:"size:512" json:"last_error,omitempty"`
}

// AssistantSession groups a multi-turn chat with the AI assistant.
type AssistantSession struct {
	Base
	UserID string `gorm:"index;size:36;not null" json:"user_id"`
	Title  string `gorm:"size:255" json:"title,omitempty"`
}

// AssistantMessage is one entry in an AssistantSession transcript.
//
// Role is "user" | "assistant" | "system".  The optional OperationID
// links a message to an action the assistant proposed (so the UI can
// offer Undo).
type AssistantMessage struct {
	Base
	SessionID   string `gorm:"index;size:36;not null" json:"session_id"`
	Role        string `gorm:"size:16;not null" json:"role"`
	Content     string `gorm:"type:text;not null" json:"content"`
	OperationID string `gorm:"size:36" json:"operation_id,omitempty"`
}

// AllModels returns the slice consumed by gorm.AutoMigrate.
func AllModels() []interface{} {
	return []interface{}{
		&User{},
		&Library{},
		&Series{},
		&Media{},
		&PlaybackHistory{},
		&Favorite{},
		&Playlist{},
		&PlaylistItem{},
		&DownloadTask{},
		&Subscription{},
		&Setting{},
		&Site{},
		&AccessLog{},
		&APIConfig{},
		&UserPermission{},
		&RefreshToken{},
		&ApiConfig{},
		&DownloadClient{},
		&NotifyChannel{},
		&TelegramBinding{},
		&STRMRecord{},
		&PlayProfile{},
		&StorageConfig{},
		&AssistantSession{},
		&AssistantMessage{},
		&RegistrationCode{},
		&SignIn{},
		&UserDevice{},
	}
}
