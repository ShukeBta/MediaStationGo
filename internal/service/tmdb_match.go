package service

import (
	"context"
	"fmt"
	"net/url"
)

func (t *TMDbProvider) GetMovieMatch(ctx context.Context, tmdbID int) (*Match, error) {
	if tmdbID <= 0 {
		return nil, nil
	}
	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, nil
	}
	base := t.resolveBaseURL(ctx)
	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("language", "zh-CN")
	u := base + "/movie/" + fmt.Sprint(tmdbID) + "?" + q.Encode()
	var r struct {
		ID               int     `json:"id"`
		Title            string  `json:"title"`
		OriginalTitle    string  `json:"original_title"`
		OriginalLanguage string  `json:"original_language"`
		Overview         string  `json:"overview"`
		PosterPath       string  `json:"poster_path"`
		BackdropPath     string  `json:"backdrop_path"`
		ReleaseDate      string  `json:"release_date"`
		VoteAverage      float32 `json:"vote_average"`
		Genres           []struct {
			Name string `json:"name"`
		} `json:"genres"`
		ProductionCountries []struct {
			Iso3166_1 string `json:"iso_3166_1"`
		} `json:"production_countries"`
		SpokenLanguages []struct {
			Iso639_1 string `json:"iso_639_1"`
		} `json:"spoken_languages"`
	}
	if err := t.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	m := &Match{
		TMDbID:       r.ID,
		MediaType:    "movie",
		Title:        r.Title,
		OriginalName: r.OriginalTitle,
		Overview:     r.Overview,
		Rating:       r.VoteAverage,
		Languages:    nonEmptyStrings(r.OriginalLanguage),
	}
	if m.Title == "" {
		m.Title = r.OriginalTitle
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
	for _, g := range r.Genres {
		m.Genres = append(m.Genres, g.Name)
	}
	for _, c := range r.ProductionCountries {
		m.Countries = append(m.Countries, c.Iso3166_1)
	}
	for _, l := range r.SpokenLanguages {
		m.Languages = append(m.Languages, l.Iso639_1)
	}
	m.Genres = deduplicate(m.Genres)
	m.Countries = deduplicate(m.Countries)
	m.Languages = deduplicate(m.Languages)
	return m, nil
}

func (t *TMDbProvider) GetTVMatch(ctx context.Context, tmdbID int) (*Match, error) {
	if tmdbID <= 0 {
		return nil, nil
	}
	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, nil
	}
	base := t.resolveBaseURL(ctx)
	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("language", "zh-CN")
	u := base + "/tv/" + fmt.Sprint(tmdbID) + "?" + q.Encode()
	var r struct {
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
		Genres           []struct {
			Name string `json:"name"`
		} `json:"genres"`
		SpokenLanguages []struct {
			Iso639_1 string `json:"iso_639_1"`
		} `json:"spoken_languages"`
	}
	if err := t.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	m := &Match{
		TMDbID:       r.ID,
		MediaType:    "tv",
		Title:        r.Name,
		OriginalName: r.OriginalName,
		Overview:     r.Overview,
		Rating:       r.VoteAverage,
		Languages:    nonEmptyStrings(r.OriginalLanguage),
		Countries:    deduplicate(r.OriginCountry),
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
	for _, g := range r.Genres {
		m.Genres = append(m.Genres, g.Name)
	}
	for _, l := range r.SpokenLanguages {
		m.Languages = append(m.Languages, l.Iso639_1)
	}
	m.Genres = deduplicate(m.Genres)
	m.Languages = deduplicate(m.Languages)
	return m, nil
}
