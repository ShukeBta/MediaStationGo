package model

import "time"

// DownloadTask 是待处理（或已完成）的 torrent / HTTP 下载。
type DownloadTask struct {
	Base
	UserID         string `gorm:"index;size:36" json:"user_id"`
	SubscriptionID string `gorm:"index;size:36" json:"subscription_id,omitempty"`
	Source         string `gorm:"size:32;not null" json:"source"` // qbittorrent / transmission / http
	URL            string `gorm:"size:2048;not null" json:"-"`
	Title          string `gorm:"size:512" json:"title,omitempty"`
	PosterURL      string `gorm:"size:2048" json:"poster_url,omitempty"`
	BackdropURL    string `gorm:"size:2048" json:"backdrop_url,omitempty"`
	Overview       string `gorm:"type:text" json:"overview,omitempty"`
	SavePath       string `gorm:"size:1024" json:"save_path"`
	MediaType      string `gorm:"size:16" json:"media_type,omitempty"`
	MediaCategory  string `gorm:"size:128" json:"media_category,omitempty"`
	// 媒体展示元数据(用于 Telegram 富通知模板等):原始片名/语言/年份/评分/类型。
	OriginalName     string  `gorm:"size:512" json:"original_name,omitempty"`
	OriginalLanguage string  `gorm:"size:32" json:"original_language,omitempty"`
	Year             int     `json:"year,omitempty"`
	Rating           float32 `json:"rating,omitempty"`
	Genres           string  `gorm:"size:255" json:"genres,omitempty"` // comma separated
	Status           string  `gorm:"size:32;default:queued" json:"status"`
	Progress         float32 `json:"progress"`

	// AllowExistingLibrary is true for subscription wash/upgrade tasks that are
	// allowed to replace an existing library item after download completion.
	AllowExistingLibrary bool `gorm:"default:false" json:"allow_existing_library,omitempty"`
}

// Subscription 是自动化规则，轮询 RSS 源并将匹配种子排队到配置的下载客户端。
type Subscription struct {
	Base
	UserID           string `gorm:"index;size:36" json:"user_id"`
	Name             string `gorm:"size:128;not null" json:"name"`
	FeedURL          string `gorm:"size:2048;not null" json:"feed_url"`
	DeliveryMode     string `gorm:"size:32;not null;default:download" json:"delivery_mode"` // download / resource_import
	LibraryID        string `gorm:"index;size:36" json:"library_id,omitempty"`
	LibraryRootID    string `gorm:"index;size:36" json:"library_root_id,omitempty"`
	ResourceSource   string `gorm:"size:32" json:"resource_source,omitempty"`
	MaxImportsPerRun int    `gorm:"not null;default:2" json:"max_imports_per_run,omitempty"`
	SeasonNumber     int    `gorm:"not null;default:1" json:"season_number,omitempty"`
	Filter           string `gorm:"size:512" json:"filter"`
	MediaType        string `gorm:"size:16" json:"media_type,omitempty"`
	MediaCategory    string `gorm:"size:128" json:"media_category,omitempty"`
	SavePath         string `gorm:"size:1024" json:"save_path,omitempty"`
	SearchMode       string `gorm:"size:16;default:keyword" json:"search_mode,omitempty"` // keyword / imdb
	IMDBID           string `gorm:"size:32" json:"imdb_id,omitempty"`
	Source           string `gorm:"size:32" json:"source,omitempty"`
	PosterURL        string `gorm:"size:2048" json:"poster_url,omitempty"`
	BackdropURL      string `gorm:"size:2048" json:"backdrop_url,omitempty"`
	Overview         string `gorm:"type:text" json:"overview,omitempty"`
	// 媒体展示元数据(用于 Telegram 富通知模板等):原始片名/语言/年份/评分/类型。
	OriginalName     string     `gorm:"size:512" json:"original_name,omitempty"`
	OriginalLanguage string     `gorm:"size:32" json:"original_language,omitempty"`
	Year             int        `json:"year,omitempty"`
	Rating           float32    `json:"rating,omitempty"`
	Genres           string     `gorm:"size:255" json:"genres,omitempty"`         // comma separated
	Resolution       string     `gorm:"size:32" json:"resolution,omitempty"`      // 2160p / 1080p / 720p / best
	Quality          string     `gorm:"size:64" json:"quality,omitempty"`         // remux / bluray / web-dl / hdtv
	Effects          string     `gorm:"size:128" json:"effects,omitempty"`        // hdr,dolby-vision,atmos
	ReleaseGroups    string     `gorm:"size:255" json:"release_groups,omitempty"` // comma separated
	ExcludeWords     string     `gorm:"size:255" json:"exclude_words,omitempty"`  // comma separated
	MinSeeders       int        `gorm:"default:0" json:"min_seeders,omitempty"`
	MaxSeeders       int        `gorm:"default:0" json:"max_seeders,omitempty"`
	MinSizeGB        float64    `gorm:"default:0" json:"min_size_gb,omitempty"`
	MaxSizeGB        float64    `gorm:"default:0" json:"max_size_gb,omitempty"`
	FreeOnly         bool       `gorm:"default:false" json:"free_only,omitempty"`
	WashEnabled      bool       `gorm:"default:false" json:"wash_enabled"`
	WashPriority     string     `gorm:"size:32" json:"wash_priority,omitempty"` // balanced / resolution / quality / effects / seeders
	TotalEpisodes    int        `gorm:"default:0" json:"total_episodes,omitempty"`
	Priority         int        `gorm:"default:50" json:"priority,omitempty"` // lower is earlier when schedulers sort later
	Enabled          bool       `gorm:"default:true" json:"enabled"`
	LastRunAt        *time.Time `json:"last_run_at,omitempty"`
	ArchivedAt       *time.Time `gorm:"index" json:"archived_at,omitempty"`
	ArchiveReason    string     `gorm:"size:255" json:"archive_reason,omitempty"`

	DownloadedEpisodes int                     `gorm:"-" json:"downloaded_episodes,omitempty"`
	LocalMediaCount    int                     `gorm:"-" json:"local_media_count,omitempty"`
	MissingEpisodes    []int                   `gorm:"-" json:"missing_episodes,omitempty"`
	InLibrary          bool                    `gorm:"-" json:"in_library"`
	ImportJobs         []SubscriptionImportJob `gorm:"-" json:"import_jobs,omitempty"`
}

// SubscriptionImportJob is the audit summary linked into the existing
// subscription history response. The authoritative task row remains
// ResourceImportJob; this is only its safe API projection.
type SubscriptionImportJob struct {
	ID                   string     `json:"id"`
	RetryOfJobID         string     `json:"retry_of_job_id,omitempty"`
	Attempt              int        `json:"attempt"`
	CandidateTitle       string     `json:"candidate_title,omitempty"`
	CandidateSource      string     `json:"candidate_source,omitempty"`
	CandidateGranularity string     `json:"candidate_granularity,omitempty"`
	SelectedEpisodes     []int      `json:"selected_episodes,omitempty"`
	MovedEpisodes        []int      `json:"moved_episodes,omitempty"`
	BlockReason          string     `json:"block_reason,omitempty"`
	Status               string     `json:"status"`
	Stage                string     `json:"stage,omitempty"`
	Outcome              string     `json:"outcome,omitempty"`
	Error                string     `json:"error,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	FinishedAt           *time.Time `json:"finished_at,omitempty"`
}
