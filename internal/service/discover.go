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
	"errors"
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
	douban       *DoubanProvider
	client       *http.Client
	images       *ImageProxy
	sectionCache *DiscoverSectionCache
}

var ErrDiscoverTMDbNotConfigured = errors.New("TMDB 推荐未配置，请在 API 设置中配置并启用 TMDB API Key")

func (d *DiscoverService) SetDouban(douban *DoubanProvider) *DiscoverService {
	d.douban = douban
	return d
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
	return tmdbMatchesToExternal(path, matches), nil
}

// TMDbSectionWindow returns one logical Discover page plus one extra item used
// by the handler to determine whether a following page exists. TMDb fixes its
// upstream page size at 20, so logical 18-item pages can span two source pages.
func (d *DiscoverService) TMDbSectionWindow(ctx context.Context, key string, page, pageSize int) ([]ExternalMediaResult, error) {
	path := tmdbDiscoverPath(key)
	if path == "" || pageSize <= 0 {
		return []ExternalMediaResult{}, nil
	}
	const sourcePageSize = 20
	sourcePage, sourceOffset := discoverWindowStart(page, pageSize, sourcePageSize)
	targetSize := pageSize + 1
	matches := make([]Match, 0, targetSize)
	for len(matches) < targetSize {
		chunk, err := d.Fetch(ctx, path, sourcePage)
		if err != nil {
			return nil, err
		}
		if sourceOffset < len(chunk) {
			remaining := targetSize - len(matches)
			available := chunk[sourceOffset:]
			if len(available) > remaining {
				available = available[:remaining]
			}
			matches = append(matches, available...)
		}
		if len(chunk) < sourcePageSize {
			break
		}
		sourcePage++
		sourceOffset = 0
	}
	return tmdbMatchesToExternal(path, matches), nil
}

func tmdbMatchesToExternal(path string, matches []Match) []ExternalMediaResult {
	mediaType := tmdbDiscoverMediaType(path)
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
			ReleaseDate:      item.ReleaseDate,
			Rating:           item.Rating,
			Genres:           item.Genres,
			Countries:        item.Countries,
			TMDbID:           item.TMDbID,
			SubscribeKeyword: buildSubscribeKeyword(item.Title, item.Year),
			SubscribeAliases: buildSubscribeAliases(item.Title, item.OriginalName, item.Year),
		})
	}
	return out
}

// TMDbItemDetail returns enriched metadata for one Discover item. Results use
// the same six-hour cache as Discover sections so repeatedly opening a detail
// does not repeatedly request TMDb.
func (d *DiscoverService) TMDbItemDetail(ctx context.Context, mediaType string, tmdbID int) (ExternalMediaResult, error) {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType != "movie" && mediaType != "tv" {
		return ExternalMediaResult{}, fmt.Errorf("unsupported tmdb media type: %s", mediaType)
	}
	if tmdbID <= 0 {
		return ExternalMediaResult{}, errors.New("invalid tmdb id")
	}
	if d == nil || d.tmdb == nil {
		return ExternalMediaResult{}, errors.New("tmdb provider is unavailable")
	}

	cacheKey := fmt.Sprintf("detail:tmdb:%s:%d", mediaType, tmdbID)
	if cached, ok := d.CachedSection(cacheKey, 1); ok && len(cached) > 0 {
		return cached[0], nil
	}

	var (
		match *Match
		err   error
	)
	if mediaType == "tv" {
		match, err = d.tmdb.GetTVMatch(ctx, tmdbID)
	} else {
		match, err = d.tmdb.GetMovieMatch(ctx, tmdbID)
	}
	if err != nil {
		return ExternalMediaResult{}, err
	}
	if match == nil || strings.TrimSpace(match.Title) == "" {
		return ExternalMediaResult{}, errors.New("tmdb detail returned no usable metadata")
	}

	item := externalMediaResultFromMatch("tmdb", mediaType, match)
	d.RememberSection(cacheKey, 1, []ExternalMediaResult{item})
	return item, nil
}

// DoubanItemDetail returns the richer subject metadata used by Discover.
// Results share the Discover cache so reopening a card does not refetch Douban.
func (d *DiscoverService) DoubanItemDetail(ctx context.Context, mediaType, doubanID string) (ExternalMediaResult, error) {
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	if mediaType != "movie" && mediaType != "tv" {
		return ExternalMediaResult{}, fmt.Errorf("unsupported douban media type: %s", mediaType)
	}
	doubanID = strings.TrimSpace(doubanID)
	if doubanID == "" {
		return ExternalMediaResult{}, errors.New("invalid douban id")
	}
	if d == nil || d.douban == nil {
		return ExternalMediaResult{}, errors.New("douban provider is unavailable")
	}

	cacheKey := fmt.Sprintf("detail:douban:%s", doubanID)
	if cached, ok := d.CachedSection(cacheKey, 1); ok && len(cached) > 0 {
		return cached[0], nil
	}

	match, err := d.douban.GetDiscoverDetailByID(ctx, doubanID)
	if err != nil {
		return ExternalMediaResult{}, err
	}
	if match == nil || strings.TrimSpace(match.Title) == "" {
		return ExternalMediaResult{}, errors.New("douban detail returned no usable metadata")
	}
	if match.MediaType == "movie" || match.MediaType == "tv" {
		mediaType = match.MediaType
	}
	item := externalMediaResultFromMatch("douban", mediaType, match)
	item.ProviderURL = "https://movie.douban.com/subject/" + url.PathEscape(doubanID) + "/"
	item.SubscribeAliases = deduplicate(append(item.SubscribeAliases, match.Aliases...))
	d.RememberSection(cacheKey, 1, []ExternalMediaResult{item})
	return item, nil
}

// fetch is the shared helper that paginates page=1 only — that's all the
// home page needs and it keeps us under TMDb's 50 rps limit.
func (d *DiscoverService) fetch(ctx context.Context, path string) ([]Match, error) {
	return d.Fetch(ctx, path)
}

// Fetch is the public entry point used by the multi-section handler.
// Optional page numbers select an upstream page; endpoint query parameters
// are preserved while authentication, language and pagination stay controlled.
func (d *DiscoverService) Fetch(ctx context.Context, path string, pages ...int) ([]Match, error) {
	if d == nil || d.tmdb == nil {
		return nil, ErrDiscoverTMDbNotConfigured
	}

	// Resolve API key from config or database
	apiKey := d.tmdb.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, ErrDiscoverTMDbNotConfigured
	}
	base := d.tmdb.resolveBaseURL(ctx)

	endpoint, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return nil, tmdbRequestFailure(path, err)
	}
	q, err := url.ParseQuery(endpoint.RawQuery)
	if err != nil {
		return nil, fmt.Errorf("tmdb %s: invalid discovery query", tmdbErrorEndpoint(path))
	}
	q.Set("api_key", apiKey)
	q.Set("language", "zh-CN")
	pageNumber := 1
	if len(pages) > 0 && pages[0] > 0 {
		pageNumber = pages[0]
	}
	q.Set("page", strconv.Itoa(pageNumber))
	endpoint.RawQuery = q.Encode()
	u := endpoint.String()

	type result struct {
		ID            int      `json:"id"`
		Title         string   `json:"title"`
		Name          string   `json:"name"`
		OriginalTitle string   `json:"original_title"`
		OriginalName  string   `json:"original_name"`
		Overview      string   `json:"overview"`
		PosterPath    string   `json:"poster_path"`
		BackdropPath  string   `json:"backdrop_path"`
		ReleaseDate   string   `json:"release_date"`
		FirstAirDate  string   `json:"first_air_date"`
		VoteAverage   float32  `json:"vote_average"`
		GenreIDs      []int    `json:"genre_ids"`
		OriginCountry []string `json:"origin_country"`
	}
	type page struct {
		Results []result `json:"results"`
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, tmdbRequestFailure(u, err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, tmdbRequestFailure(u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("tmdb %s: HTTP %d", tmdbErrorEndpoint(u), resp.StatusCode)
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
			Genres:       tmdbGenreNames(tmdbDiscoverMediaType(path), r.GenreIDs),
			Countries:    r.OriginCountry,
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
		m.ReleaseDate = normalizeReleaseDate(date)
		if len(date) >= 4 {
			_, _ = fmt.Sscanf(date[:4], "%d", &m.Year)
		}
		out = append(out, m)
	}
	return out, nil
}
