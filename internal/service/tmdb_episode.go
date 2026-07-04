package service

import (
	"context"
	"fmt"
	"net/url"
)

func (t *TMDbProvider) GetTVEpisodeDetails(ctx context.Context, tmdbID, season, episode int) (*TMDbEpisodeDetails, error) {
	if tmdbID <= 0 || episode <= 0 {
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
	u := base + "/tv/" + fmt.Sprint(tmdbID) + "/season/" + fmt.Sprint(season) + "/episode/" + fmt.Sprint(episode) + "?" + q.Encode()
	var r struct {
		Name        string  `json:"name"`
		Overview    string  `json:"overview"`
		StillPath   string  `json:"still_path"`
		AirDate     string  `json:"air_date"`
		VoteAverage float32 `json:"vote_average"`
		Runtime     int     `json:"runtime"`
	}
	if err := t.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	details := &TMDbEpisodeDetails{
		Name:     r.Name,
		Overview: r.Overview,
		Rating:   r.VoteAverage,
		Runtime:  r.Runtime,
	}
	if r.StillPath != "" {
		details.StillURL = t.imgCDN + "/w500" + r.StillPath
	}
	if len(r.AirDate) >= 4 {
		_, _ = fmt.Sscanf(r.AirDate[:4], "%d", &details.AirYear)
	}
	return details, nil
}

func (t *TMDbProvider) GetTVEpisodeCount(ctx context.Context, tmdbID int) (int, error) {
	if tmdbID <= 0 {
		return 0, nil
	}
	apiKey := t.resolveAPIKey(ctx)
	if apiKey == "" {
		return 0, nil
	}
	base := t.resolveBaseURL(ctx)
	q := url.Values{}
	q.Set("api_key", apiKey)
	q.Set("language", "zh-CN")
	u := base + "/tv/" + fmt.Sprint(tmdbID) + "?" + q.Encode()
	var r struct {
		NumberOfEpisodes int `json:"number_of_episodes"`
	}
	if err := t.getJSON(ctx, u, &r); err != nil {
		return 0, err
	}
	return r.NumberOfEpisodes, nil
}
