// Package service — TheTVDB v4 provider.
//
// TheTVDBProvider implements two methods used by the scraper for TV /
// anime libraries:
//
//	Login()                    -> exchanges secrets.thetvdb_api_key for
//	                               a JWT (cached for 24h).
//	SearchSeries(query)        -> /search?query=...&type=series
//
// The provider is enabled iff secrets.thetvdb_api_key is non-empty. When
// disabled every method returns nil, nil so the scraper orchestrator can
// gracefully fall through.
package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// TheTVDBProvider talks to https://api4.thetvdb.com/v4.
type TheTVDBProvider struct {
	cfg    *config.Config
	log    *zap.Logger
	client *http.Client

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewTheTVDBProvider is the constructor.
func NewTheTVDBProvider(cfg *config.Config, log *zap.Logger) *TheTVDBProvider {
	return &TheTVDBProvider{
		cfg:    cfg,
		log:    log,
		client: NewExternalHTTPClient(15 * time.Second),
	}
}

// Enabled reports whether an API key is present.
func (t *TheTVDBProvider) Enabled() bool { return t.cfg.Secrets.TheTVDBAPIKey != "" }

// Login fetches a fresh JWT, cached for 24h.
func (t *TheTVDBProvider) Login(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if time.Now().Before(t.tokenExp) && t.token != "" {
		return t.token, nil
	}
	body, _ := json.Marshal(map[string]string{"apikey": t.cfg.Secrets.TheTVDBAPIKey})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://api4.thetvdb.com/v4/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("thetvdb login: %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	t.token = out.Data.Token
	t.tokenExp = time.Now().Add(24 * time.Hour)
	return t.token, nil
}

// SearchSeries returns the top match for a TV / anime query, or nil.
func (t *TheTVDBProvider) SearchSeries(ctx context.Context, query string) (*Match, error) {
	if !t.Enabled() || query == "" {
		return nil, nil
	}
	tok, err := t.Login(ctx)
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("https://api4.thetvdb.com/v4/search?query=%s&type=series",
		urlEscape(query))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("thetvdb search: %d", resp.StatusCode)
	}

	type entry struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Overview string `json:"overview"`
		Image    string `json:"image_url"`
		Year     string `json:"year"`
	}
	type page struct {
		Data []entry `json:"data"`
	}
	var p page
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return nil, err
	}
	if len(p.Data) == 0 {
		return nil, nil
	}
	r := p.Data[0]
	m := &Match{
		TheTVDBID: r.ID,
		MediaType: "tv",
		Title:     r.Name,
		Overview:  r.Overview,
		PosterURL: r.Image,
	}
	if len(r.Year) >= 4 {
		_, _ = fmt.Sscanf(r.Year[:4], "%d", &m.Year)
	}
	return m, nil
}

func (t *TheTVDBProvider) GetSeriesEpisodeCount(ctx context.Context, seriesID string) (int, error) {
	if !t.Enabled() || strings.TrimSpace(seriesID) == "" {
		return 0, nil
	}
	tok, err := t.Login(ctx)
	if err != nil {
		return 0, err
	}
	id := normalizeTheTVDBSeriesID(seriesID)
	u := fmt.Sprintf("https://api4.thetvdb.com/v4/series/%s/extended?meta=episodes", urlEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := t.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("thetvdb series detail: %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			Episodes []struct {
				ID           int    `json:"id"`
				Name         string `json:"name"`
				Number       int    `json:"number"`
				SeasonNumber int    `json:"seasonNumber"`
				Type         string `json:"type"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	count := 0
	for _, episode := range out.Data.Episodes {
		if episode.SeasonNumber == 0 || strings.EqualFold(episode.Type, "special") {
			continue
		}
		count++
	}
	if count > 0 {
		return count, nil
	}
	return len(out.Data.Episodes), nil
}

func (t *TheTVDBProvider) GetSeriesMatchByID(ctx context.Context, seriesID string) (*Match, error) {
	if !t.Enabled() || strings.TrimSpace(seriesID) == "" {
		return nil, nil
	}
	tok, err := t.Login(ctx)
	if err != nil {
		return nil, err
	}
	id := normalizeTheTVDBSeriesID(seriesID)
	u := fmt.Sprintf("https://api4.thetvdb.com/v4/series/%s/extended", urlEscape(id))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("thetvdb series detail: %d", resp.StatusCode)
	}
	var out struct {
		Data struct {
			ID         int    `json:"id"`
			Name       string `json:"name"`
			Overview   string `json:"overview"`
			Image      string `json:"image"`
			ImageURL   string `json:"image_url"`
			FirstAired string `json:"firstAired"`
			Year       string `json:"year"`
			Score      int    `json:"score"`
			Average    string `json:"averageRuntime"`
			Genres     []struct {
				Name string `json:"name"`
			} `json:"genres"`
			OriginalCountry  string `json:"originalCountry"`
			OriginalLanguage string `json:"originalLanguage"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	idValue := id
	if out.Data.ID > 0 {
		idValue = fmt.Sprint(out.Data.ID)
	}
	m := &Match{
		TheTVDBID: idValue,
		MediaType: "tv",
		Title:     out.Data.Name,
		Overview:  out.Data.Overview,
		PosterURL: firstNonEmpty(out.Data.Image, out.Data.ImageURL),
		Languages: nonEmptyStrings(out.Data.OriginalLanguage),
		Countries: nonEmptyStrings(out.Data.OriginalCountry),
	}
	if out.Data.Score > 0 {
		m.Rating = float32(out.Data.Score) / 10
	}
	yearText := firstNonEmpty(out.Data.Year, out.Data.FirstAired)
	if len(yearText) >= 4 {
		_, _ = fmt.Sscanf(yearText[:4], "%d", &m.Year)
	}
	for _, genre := range out.Data.Genres {
		if strings.TrimSpace(genre.Name) != "" {
			m.Genres = append(m.Genres, genre.Name)
		}
	}
	return m, nil
}

func normalizeTheTVDBSeriesID(seriesID string) string {
	seriesID = strings.TrimSpace(seriesID)
	if idx := strings.LastIndex(seriesID, "-"); idx >= 0 && idx+1 < len(seriesID) {
		return seriesID[idx+1:]
	}
	return seriesID
}

// urlEscape is a tiny replacement for net/url.QueryEscape kept inline so
// the file does not pull a second import for one call.
func urlEscape(s string) string {
	out := make([]byte, 0, len(s)*3)
	for _, r := range []byte(s) {
		switch {
		case r >= '0' && r <= '9',
			r >= 'A' && r <= 'Z',
			r >= 'a' && r <= 'z',
			r == '-', r == '_', r == '.', r == '~':
			out = append(out, r)
		default:
			out = append(out, '%')
			const hex = "0123456789ABCDEF"
			out = append(out, hex[r>>4], hex[r&15])
		}
	}
	return string(out)
}
