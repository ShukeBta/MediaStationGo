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
	q.Set("append_to_response", "credits")
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
		Runtime          int     `json:"runtime"`
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
		Credits struct {
			Cast []tmdbCreditCast `json:"cast"`
		} `json:"credits"`
	}
	if err := t.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	m := &Match{
		TMDbID:          r.ID,
		MediaType:       "movie",
		Title:           r.Title,
		OriginalName:    r.OriginalTitle,
		Overview:        r.Overview,
		Rating:          r.VoteAverage,
		DurationMinutes: r.Runtime,
		Languages:       nonEmptyStrings(r.OriginalLanguage),
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
	m.People = topTMDbPeople(r.Credits.Cast, t.imgCDN)
	m.Actors = personMetadataNames(m.People)
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
	q.Set("append_to_response", "credits,alternative_titles,translations")
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
		EpisodeRunTime   []int    `json:"episode_run_time"`
		NumberOfSeasons  int      `json:"number_of_seasons"`
		NumberOfEpisodes int      `json:"number_of_episodes"`
		Seasons          []struct {
			SeasonNumber int    `json:"season_number"`
			Name         string `json:"name"`
			EpisodeCount int    `json:"episode_count"`
			AirDate      string `json:"air_date"`
		} `json:"seasons"`
		AlternativeTitles struct {
			Results []struct {
				Title string `json:"title"`
			} `json:"results"`
		} `json:"alternative_titles"`
		Translations struct {
			Translations []struct {
				Data struct {
					Name string `json:"name"`
				} `json:"data"`
			} `json:"translations"`
		} `json:"translations"`
		VoteAverage float32 `json:"vote_average"`
		Genres      []struct {
			Name string `json:"name"`
		} `json:"genres"`
		SpokenLanguages []struct {
			Iso639_1 string `json:"iso_639_1"`
		} `json:"spoken_languages"`
		Credits struct {
			Cast []tmdbCreditCast `json:"cast"`
		} `json:"credits"`
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
		TMDbSeries: &TMDbSeriesSummary{
			TMDbID:       r.ID,
			Title:        r.Name,
			SeasonCount:  r.NumberOfSeasons,
			EpisodeCount: r.NumberOfEpisodes,
			Seasons:      make([]TMDbSeasonSummary, 0, len(r.Seasons)),
		},
	}
	for _, season := range r.Seasons {
		if season.SeasonNumber < 0 {
			continue
		}
		m.TMDbSeries.Seasons = append(m.TMDbSeries.Seasons, TMDbSeasonSummary{
			SeasonNum:    season.SeasonNumber,
			Name:         season.Name,
			EpisodeCount: season.EpisodeCount,
			AirDate:      normalizeReleaseDate(season.AirDate),
		})
	}
	if m.TMDbSeries.SeasonCount <= 0 {
		for _, season := range m.TMDbSeries.Seasons {
			if season.SeasonNum > 0 {
				m.TMDbSeries.SeasonCount++
			}
		}
	}
	if m.TMDbSeries.EpisodeCount <= 0 {
		for _, season := range m.TMDbSeries.Seasons {
			m.TMDbSeries.EpisodeCount += season.EpisodeCount
		}
	}
	for _, runtime := range r.EpisodeRunTime {
		if runtime > 0 {
			m.DurationMinutes = runtime
			break
		}
	}
	for _, alias := range r.AlternativeTitles.Results {
		m.Aliases = append(m.Aliases, alias.Title)
	}
	for _, translation := range r.Translations.Translations {
		m.Aliases = append(m.Aliases, translation.Data.Name)
	}
	m.Aliases = deduplicate(m.Aliases)
	if m.Title == "" {
		m.Title = r.OriginalName
	}
	m.TMDbSeries.Title = m.Title
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
	m.People = topTMDbPeople(r.Credits.Cast, t.imgCDN)
	m.Actors = personMetadataNames(m.People)
	return m, nil
}
