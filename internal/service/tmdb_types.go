package service

// Match describes a successful metadata match. The same struct is reused
// across providers; provider-specific IDs sit side-by-side so the scraper
// orchestrator can write them all into a single update.
type Match struct {
	TMDbID        int      `json:"tmdb_id"`
	BangumiID     int      `json:"bangumi_id"`
	DoubanID      string   `json:"douban_id,omitempty"`
	TheTVDBID     string   `json:"thetvdb_id,omitempty"`
	MediaType     string   `json:"media_type,omitempty"`
	Title         string   `json:"title"`
	OriginalName  string   `json:"original_name,omitempty"`
	Overview      string   `json:"overview"`
	PosterURL     string   `json:"poster_url"`
	BackdropURL   string   `json:"backdrop_url"`
	Year          int      `json:"year"`
	ReleaseDate   string   `json:"release_date,omitempty"`
	Rating        float32  `json:"rating"`
	Languages     []string `json:"languages,omitempty"`
	Countries     []string `json:"countries,omitempty"`
	Genres        []string `json:"genres,omitempty"`
	NSFW          bool     `json:"nsfw,omitempty"`
	SearchKeyword string   `json:"-"`
}

// TMDbEpisodeDetails holds per-episode metadata from /tv/{id}/season/{season}/episode/{episode}.
type TMDbEpisodeDetails struct {
	Name     string
	Overview string
	StillURL string
	AirYear  int
	Rating   float32
	Runtime  int
}

// TMDbDetails holds extended metadata from the /movie/{id} or /tv/{id} endpoints.
type TMDbDetails struct {
	Languages []string `json:"languages"`
	Countries []string `json:"countries"`
	Genres    []string `json:"genres"`
}
