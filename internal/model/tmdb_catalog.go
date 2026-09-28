package model

import "time"

// TMDbCatalogItem is provider data, independent of playable media files.
// Snapshot stays plain JSON text so SQLite and PostgreSQL share one schema.
type TMDbCatalogItem struct {
	Key          string    `gorm:"primaryKey;size:128" json:"key"`
	Kind         string    `gorm:"size:16;index" json:"kind"`
	RootID       int       `gorm:"index:idx_tmdb_catalog_root_season,priority:1" json:"root_id"`
	TMDbID       int       `json:"tmdb_id"`
	SeasonNum    int       `gorm:"index:idx_tmdb_catalog_root_season,priority:2" json:"season_num"`
	EpisodeNum   int       `json:"episode_num"`
	Title        string    `gorm:"type:text" json:"title"`
	Overview     string    `gorm:"type:text" json:"overview"`
	ReleaseDate  string    `gorm:"size:10" json:"release_date"`
	PosterURL    string    `gorm:"type:text" json:"poster_url"`
	StillURL     string    `gorm:"type:text" json:"still_url"`
	EpisodeCount int       `json:"episode_count"`
	Snapshot     string    `gorm:"type:text" json:"-"`
	Complete     bool      `json:"complete"`
	Expanded     bool      `json:"-"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// TMDbCatalogJob persists cooldown, retry and lease state across restarts.
type TMDbCatalogJob struct {
	Key        string     `gorm:"primaryKey;size:128" json:"key"`
	Status     string     `gorm:"size:16;index" json:"status"`
	DueAt      time.Time  `gorm:"index" json:"due_at"`
	CheckedAt  *time.Time `json:"checked_at"`
	Attempts   int        `json:"attempts"`
	LastError  string     `gorm:"type:text" json:"last_error,omitempty"`
	LeaseToken string     `gorm:"size:36" json:"-"`
	LeaseUntil *time.Time `json:"-"`
}
