package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
)

type tmdbMovieSearchResult struct {
	ID               int     `json:"id"`
	Title            string  `json:"title"`
	OriginalTitle    string  `json:"original_title"`
	OriginalLanguage string  `json:"original_language"`
	Overview         string  `json:"overview"`
	PosterPath       string  `json:"poster_path"`
	BackdropPath     string  `json:"backdrop_path"`
	ReleaseDate      string  `json:"release_date"`
	VoteAverage      float32 `json:"vote_average"`
	GenreIDs         []int   `json:"genre_ids"`
}

type tmdbTVSearchResult struct {
	ID               int      `json:"id"`
	Name             string   `json:"name"`
	OriginalName     string   `json:"original_name"`
	OriginalLanguage string   `json:"original_language"`
	OriginCountry    []string `json:"origin_country"`
	Overview         string   `json:"overview"`
	PosterPath       string   `json:"poster_path"`
	BackdropPath     string   `json:"backdrop_path"`
	FirstAirDate     string   `json:"first_air_date"`
	VoteAverage      float32  `json:"vote_average"`
	GenreIDs         []int    `json:"genre_ids"`
}

// SearchMovie issues `/search/movie` and returns the best match, or nil
// when no result is found. The `year` argument is optional (0 = any).
func (t *TMDbProvider) SearchMovie(ctx context.Context, query string, year int) (*Match, error) {
	matches, err := t.SearchMovieCandidates(ctx, query, year)
	if err != nil || len(matches) == 0 {
		return nil, err
	}
	return matches[0], nil
}

// SearchMovieCandidates returns the first localized TMDb result page. Manual
// scrape exposes the full page; automatic scrape ranks the same candidates.
func (t *TMDbProvider) SearchMovieCandidates(ctx context.Context, query string, year int) ([]*Match, error) {
	return t.searchMovieCandidates(ctx, query, year, "zh-CN")
}

func (t *TMDbProvider) searchMovieCandidates(ctx context.Context, query string, year int, language string) ([]*Match, error) {
	if query == "" {
		return nil, errors.New("empty query")
	}

	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, nil
	}
	base := t.resolveBaseURL(ctx)

	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("query", query)
	q.Set("language", language)
	q.Set("include_adult", "false")
	if year > 0 {
		q.Set("year", fmt.Sprintf("%d", year))
	}
	u := base + "/search/movie?" + q.Encode()

	type page struct {
		Results []tmdbMovieSearchResult `json:"results"`
	}

	var p page
	if err := t.getJSON(ctx, u, &p); err != nil {
		return nil, err
	}
	if len(p.Results) == 0 {
		return nil, nil
	}
	out := make([]*Match, 0, len(p.Results))
	for _, r := range p.Results {
		match := t.movieSearchResultToMatch(r)
		match.SearchKeyword = query
		out = append(out, match)
	}
	return out, nil
}

func (t *TMDbProvider) movieSearchResultToMatch(r tmdbMovieSearchResult) *Match {
	m := &Match{
		TMDbID:       r.ID,
		MediaType:    "movie",
		Title:        r.Title,
		OriginalName: r.OriginalTitle,
		Overview:     r.Overview,
		Rating:       r.VoteAverage,
		Languages:    nonEmptyStrings(r.OriginalLanguage),
		Genres:       genreIDStrings(r.GenreIDs),
	}
	if r.PosterPath != "" {
		m.PosterURL = t.imgCDN + "/w500" + r.PosterPath
	}
	if r.BackdropPath != "" {
		m.BackdropURL = t.imgCDN + "/w1280" + r.BackdropPath
	}
	m.ReleaseDate = normalizeReleaseDate(r.ReleaseDate)
	if len(r.ReleaseDate) >= 4 {
		_, _ = fmt.Sscanf(r.ReleaseDate[:4], "%d", &m.Year)
	}
	return m
}

// SearchTV issues `/search/tv` and returns the best match. Used by anime /
// tv libraries before falling back to SearchMovie.
func (t *TMDbProvider) SearchTV(ctx context.Context, query string, year int) (*Match, error) {
	matches, err := t.SearchTVCandidates(ctx, query, year)
	if err != nil || len(matches) == 0 {
		return nil, err
	}
	return matches[0], nil
}

// SearchTVCandidates returns the first TMDb TV result page for manual scrape.
func (t *TMDbProvider) SearchTVCandidates(ctx context.Context, query string, year int) ([]*Match, error) {
	return t.searchTVCandidates(ctx, query, year, "zh-CN")
}

func (t *TMDbProvider) searchTVCandidates(ctx context.Context, query string, year int, language string) ([]*Match, error) {
	if query == "" {
		return nil, errors.New("empty query")
	}

	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, nil
	}
	base := t.resolveBaseURL(ctx)

	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("query", query)
	q.Set("language", language)
	q.Set("include_adult", "false")
	if year > 0 {
		q.Set("first_air_date_year", fmt.Sprintf("%d", year))
	}
	u := base + "/search/tv?" + q.Encode()

	type page struct {
		Results []tmdbTVSearchResult `json:"results"`
	}

	var p page
	if err := t.getJSON(ctx, u, &p); err != nil {
		return nil, err
	}
	if len(p.Results) == 0 {
		return nil, nil
	}
	out := make([]*Match, 0, len(p.Results))
	for _, r := range p.Results {
		match := t.tvSearchResultToMatch(r)
		match.SearchKeyword = query
		out = append(out, match)
	}
	return out, nil
}

func (t *TMDbProvider) tvSearchResultToMatch(r tmdbTVSearchResult) *Match {
	m := &Match{
		TMDbID:       r.ID,
		MediaType:    "tv",
		Title:        r.Name,
		OriginalName: r.OriginalName,
		Overview:     r.Overview,
		Rating:       r.VoteAverage,
		Languages:    nonEmptyStrings(r.OriginalLanguage),
		Countries:    deduplicate(r.OriginCountry),
		Genres:       genreIDStrings(r.GenreIDs),
	}
	if m.Title == "" {
		m.Title = r.OriginalName
	}
	if r.PosterPath != "" {
		m.PosterURL = t.imgCDN + "/w500" + r.PosterPath
	}
	if r.BackdropPath != "" {
		m.BackdropURL = t.imgCDN + "/w1280" + r.BackdropPath
	}
	m.ReleaseDate = normalizeReleaseDate(r.FirstAirDate)
	if len(r.FirstAirDate) >= 4 {
		_, _ = fmt.Sscanf(r.FirstAirDate[:4], "%d", &m.Year)
	}
	return m
}
