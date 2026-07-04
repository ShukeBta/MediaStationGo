// Package service — TMDb discovery (trending / popular).
//
// DiscoverService surfaces curated lists from TMDb so the React home
// page can show "Trending" and "Popular" rails alongside the user's own
// library. All methods gracefully no-op when the TMDb provider is
// disabled.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// DiscoverService talks to TMDb's /trending and /movie/popular endpoints.
type DiscoverService struct {
	log          *zap.Logger
	tmdb         *TMDbProvider
	client       *http.Client
	images       *ImageProxy
	sectionCache *DiscoverSectionCache
}

// NewDiscoverService is the constructor.
func NewDiscoverService(log *zap.Logger, tmdb *TMDbProvider) *DiscoverService {
	return &DiscoverService{
		log:          log,
		tmdb:         tmdb,
		client:       NewExternalHTTPClient(15 * time.Second),
		sectionCache: NewDiscoverSectionCache(6 * time.Hour),
	}
}

// Trending returns the daily trending movies (TMDb /trending/movie/day).
func (d *DiscoverService) Trending(ctx context.Context) ([]Match, error) {
	return d.fetch(ctx, "/trending/movie/day")
}

// Popular returns the popular movies list (TMDb /movie/popular).
func (d *DiscoverService) Popular(ctx context.Context) ([]Match, error) {
	return d.fetch(ctx, "/movie/popular")
}

// TMDbSection returns one TMDb rail converted to the common external
// discovery shape used by the multi-source Discover page.
func (d *DiscoverService) TMDbSection(ctx context.Context, key string, pages ...int) ([]ExternalMediaResult, error) {
	path := tmdbDiscoverPath(key)
	if path == "" {
		return []ExternalMediaResult{}, nil
	}
	matches, err := d.Fetch(ctx, path, pages...)
	if err != nil {
		return nil, err
	}
	mediaType := "movie"
	if strings.Contains(path, "/tv/") {
		mediaType = "tv"
	}
	out := make([]ExternalMediaResult, 0, len(matches))
	for _, item := range matches {
		out = append(out, ExternalMediaResult{
			Source:           "tmdb",
			MediaType:        mediaType,
			Title:            item.Title,
			OriginalName:     item.OriginalName,
			Overview:         item.Overview,
			PosterURL:        item.PosterURL,
			BackdropURL:      item.BackdropURL,
			Year:             item.Year,
			Rating:           item.Rating,
			TMDbID:           item.TMDbID,
			SubscribeKeyword: buildSubscribeKeyword(item.Title, item.Year),
			SubscribeAliases: buildSubscribeAliases(item.Title, item.OriginalName, item.Year),
		})
	}
	return out, nil
}

// fetch is the shared helper that paginates page=1 only — that's all the
// home page needs and it keeps us under TMDb's 50 rps limit.
func (d *DiscoverService) fetch(ctx context.Context, path string) ([]Match, error) {
	return d.Fetch(ctx, path)
}

// Fetch is the public entry point used by the multi-section handler.
// It paginates page=1 only — that's all the home page needs and it
// keeps us under TMDb's 50 rps limit.
func (d *DiscoverService) Fetch(ctx context.Context, path string, pages ...int) ([]Match, error) {
	if d.tmdb == nil {
		return nil, nil
	}

	// Resolve API key from config or database
	apiKey := d.tmdb.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, nil
	}
	base := d.tmdb.resolveBaseURL(ctx)

	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("language", "zh-CN")
	pageNumber := 1
	if len(pages) > 0 && pages[0] > 0 {
		pageNumber = pages[0]
	}
	q.Set("page", strconv.Itoa(pageNumber))
	u := base + path + "?" + q.Encode()

	type result struct {
		ID            int     `json:"id"`
		Title         string  `json:"title"`
		Name          string  `json:"name"`
		OriginalTitle string  `json:"original_title"`
		OriginalName  string  `json:"original_name"`
		Overview      string  `json:"overview"`
		PosterPath    string  `json:"poster_path"`
		BackdropPath  string  `json:"backdrop_path"`
		ReleaseDate   string  `json:"release_date"`
		FirstAirDate  string  `json:"first_air_date"`
		VoteAverage   float32 `json:"vote_average"`
	}
	type page struct {
		Results []result `json:"results"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tmdb %s: %d", path, resp.StatusCode)
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	out := make([]Match, 0, len(p.Results))
	for _, r := range p.Results {
		title := r.Title
		if title == "" {
			title = r.Name
		}
		m := Match{
			TMDbID:       r.ID,
			Title:        title,
			OriginalName: firstNonEmpty(r.OriginalTitle, r.OriginalName),
			Overview:     r.Overview,
			Rating:       r.VoteAverage,
		}
		if r.PosterPath != "" {
			m.PosterURL = d.tmdb.imgCDN + "/w500" + r.PosterPath
		}
		if r.BackdropPath != "" {
			m.BackdropURL = d.tmdb.imgCDN + "/w1280" + r.BackdropPath
		}
		date := r.ReleaseDate
		if date == "" {
			date = r.FirstAirDate
		}
		if len(date) >= 4 {
			_, _ = fmt.Sscanf(date[:4], "%d", &m.Year)
		}
		out = append(out, m)
	}
	return out, nil
}

func tmdbDiscoverPath(key string) string {
	switch key {
	case "tmdb_trending_day", "trending_day":
		return "/trending/movie/day"
	case "tmdb_trending_week", "trending_week":
		return "/trending/movie/week"
	case "tmdb_latest_movie", "latest_movie":
		return "/movie/now_playing"
	case "tmdb_latest_tv", "latest_tv":
		return "/tv/on_the_air"
	case "tmdb_popular_movie", "popular_movie":
		return "/movie/popular"
	case "tmdb_popular_tv", "popular_tv":
		return "/tv/popular"
	case "tmdb_top_rated_movie", "top_rated_movie":
		return "/movie/top_rated"
	case "tmdb_upcoming_movie", "upcoming_movie":
		return "/movie/upcoming"
	default:
		return ""
	}
}
