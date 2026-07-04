package service

import (
	"context"
	"fmt"
	"net/url"

	"go.uber.org/zap"
)

// GetDetails fetches extended metadata for a TMDb ID.
// It calls /movie/{id} or /tv/{id} with append_to_response=genres
// and extracts languages, production countries, and genres.
// mediaType should be "movie" or "tv".
func (t *TMDbProvider) GetDetails(ctx context.Context, tmdbID int, mediaType string) (*TMDbDetails, error) {
	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return nil, fmt.Errorf("tmdb: no API key available")
	}
	base := t.resolveBaseURL(ctx)

	path := "/movie/" + fmt.Sprint(tmdbID)
	if mediaType == "tv" {
		path = "/tv/" + fmt.Sprint(tmdbID)
	}

	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("language", "zh-CN")
	q.Set("append_to_response", "genres")
	u := base + path + "?" + q.Encode()

	// Response structs for /movie/{id} and /tv/{id}
	type genre struct {
		Name string `json:"name"`
	}
	type movieResult struct {
		OriginalLanguage    string `json:"original_language"`
		ProductionCountries []struct {
			Iso3166_1 string `json:"iso_3166_1"`
		} `json:"production_countries"`
		SpokenLanguages []struct {
			Iso639_1 string `json:"iso_639_1"`
		} `json:"spoken_languages"`
		Genres []genre `json:"genres"`
	}
	type tvResult struct {
		OriginCountry   []string `json:"origin_country"`
		SpokenLanguages []struct {
			Iso639_1 string `json:"iso_639_1"`
		} `json:"spoken_languages"`
		Genres []genre `json:"genres"`
	}

	var (
		languages []string
		countries []string
		genres    []string
	)

	if mediaType == "tv" {
		var r tvResult
		if err := t.getJSON(ctx, u, &r); err != nil {
			return nil, err
		}
		// Spoken languages
		for _, l := range r.SpokenLanguages {
			languages = append(languages, l.Iso639_1)
		}
		// Origin countries
		countries = append(countries, r.OriginCountry...)
		// Genres
		for _, g := range r.Genres {
			genres = append(genres, g.Name)
		}
	} else {
		var r movieResult
		if err := t.getJSON(ctx, u, &r); err != nil {
			return nil, err
		}
		// Original language
		if r.OriginalLanguage != "" {
			languages = append(languages, r.OriginalLanguage)
		}
		// Spoken languages
		for _, l := range r.SpokenLanguages {
			languages = append(languages, l.Iso639_1)
		}
		// Production countries
		for _, c := range r.ProductionCountries {
			countries = append(countries, c.Iso3166_1)
		}
		// Genres
		for _, g := range r.Genres {
			genres = append(genres, g.Name)
		}
	}

	// Deduplicate
	languages = deduplicate(languages)
	countries = deduplicate(countries)
	genres = deduplicate(genres)

	t.log.Debug("tmdb: getDetails",
		zap.Int("tmdb_id", tmdbID),
		zap.String("type", mediaType),
		zap.Strings("languages", languages),
		zap.Strings("countries", countries),
		zap.Strings("genres", genres),
	)

	return &TMDbDetails{
		Languages: languages,
		Countries: countries,
		Genres:    genres,
	}, nil
}
