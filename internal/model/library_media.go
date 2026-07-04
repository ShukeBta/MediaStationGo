package model

// Library 表示一个逻辑媒体库。Path 保留为兼容字段，指向第一条 LibraryRoot。
type Library struct {
	Base
	Name    string        `gorm:"size:128;not null" json:"name"`
	Path    string        `gorm:"size:1024;not null" json:"path"`
	Type    string        `gorm:"size:16;not null;default:movie" json:"type"` // movie / tv / anime / music
	Enabled bool          `gorm:"default:true" json:"enabled"`
	Roots   []LibraryRoot `gorm:"foreignKey:LibraryID" json:"roots,omitempty"`
}

// LibraryRoot 是逻辑媒体库下的一条真实物理/挂载路径。
type LibraryRoot struct {
	Base
	LibraryID string `gorm:"index;size:36;not null" json:"library_id"`
	Name      string `gorm:"size:128" json:"name,omitempty"`
	Path      string `gorm:"size:1024;not null" json:"path"`
	Enabled   bool   `gorm:"default:true" json:"enabled"`
	SortOrder int    `gorm:"default:0" json:"sort_order"`
}

// Media 是单个可播放项。剧集链接到 SeriesID；电影 SeriesID == ""。
type Media struct {
	Base
	LibraryID     string  `gorm:"index;size:36" json:"library_id"`
	LibraryRootID string  `gorm:"index;size:36" json:"library_root_id,omitempty"`
	SeriesID      string  `gorm:"index;size:128" json:"series_id,omitempty"`
	Title         string  `gorm:"size:255;not null" json:"title"`
	OriginalName  string  `gorm:"size:255" json:"original_name,omitempty"`
	EpisodeTitle  string  `gorm:"size:255" json:"episode_title,omitempty"`
	Path          string  `gorm:"uniqueIndex;size:1024;not null" json:"path"`
	RelativePath  string  `gorm:"size:1024" json:"relative_path,omitempty"`
	SizeBytes     int64   `json:"size_bytes"`
	DurationSec   int     `json:"duration_sec"`
	Width         int     `json:"width"`
	Height        int     `json:"height"`
	VideoCodec    string  `gorm:"size:32" json:"video_codec,omitempty"`
	AudioCodec    string  `gorm:"size:32" json:"audio_codec,omitempty"`
	Container     string  `gorm:"size:128" json:"container,omitempty"`
	PosterURL     string  `gorm:"size:1024" json:"poster_url,omitempty"`
	BackdropURL   string  `gorm:"size:1024" json:"backdrop_url,omitempty"`
	Overview      string  `gorm:"type:text" json:"overview,omitempty"`
	Rating        float32 `json:"rating"`
	Year          int     `json:"year"`
	ReleaseDate   string  `gorm:"size:10;index" json:"release_date,omitempty"`
	SeasonNum     int     `json:"season_num"`
	EpisodeNum    int     `json:"episode_num"`
	ScrapeStatus  string  `gorm:"size:16;default:pending" json:"scrape_status"`
	TMDbID        int     `json:"tmdb_id"`
	BangumiID     int     `json:"bangumi_id"`
	DoubanID      string  `gorm:"column:douban_id;size:32" json:"douban_id,omitempty"`
	TheTVDBID     string  `gorm:"column:thetvdb_id;size:64" json:"thetvdb_id,omitempty"`
	Languages     string  `gorm:"size:64"  json:"languages,omitempty"` // 逗号分隔的 ISO 639-1 代码，如 "zh,en"
	Countries     string  `gorm:"size:128" json:"countries,omitempty"` // 逗号分隔的 ISO 3166-1，如 "CN,US"
	Genres        string  `gorm:"type:text" json:"genres,omitempty"`   // 逗号分隔的类型名，如 "Action,Animation"
	NSFW          bool    `gorm:"default:false" json:"nsfw"`

	// STRMURL is the indirection target for .strm files: when present the
	// stream handler redirects to it instead of opening the local file.
	// Used to expose WebDAV / Alist / S3 / HTTP direct links as media items.
	STRMURL string `gorm:"size:2048" json:"strm_url,omitempty"`

	LibraryName string `gorm:"-" json:"library_name,omitempty"`
	LibraryPath string `gorm:"-" json:"library_path,omitempty"`

	DisplayLibraryID   string `gorm:"-" json:"display_library_id,omitempty"`
	DisplayLibraryName string `gorm:"-" json:"display_library_name,omitempty"`
	DisplayLibraryPath string `gorm:"-" json:"display_library_path,omitempty"`

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
	DuplicateOf string `gorm:"size:128" json:"duplicate_of,omitempty"`
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
	DoubanID    string  `gorm:"column:douban_id;size:32" json:"douban_id,omitempty"`
	TheTVDBID   string  `gorm:"column:thetvdb_id;size:64" json:"thetvdb_id,omitempty"`
}
