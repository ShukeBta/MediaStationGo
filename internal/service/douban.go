// Package service — Douban (豆瓣) metadata provider.
//
// Douban is the dominant Chinese movie/TV rating and metadata site. Its
// unofficial API returns rich Chinese-language titles, overviews, ratings
// and poster URLs. A valid Douban cookie is required to avoid IP bans.
//
// We use the search endpoint at:
//
//	https://movie.douban.com/j/subject_suggest?q=...
//
// And the detail endpoint at:
//
//	https://movie.douban.com/j/subject_abstract?subject_id=...
//
// The provider is used as a supplemental source: after TMDb matches we
// attempt a Douban lookup to grab a localized Chinese title + overview.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// DoubanProvider talks to the unofficial Douban movie API.
type DoubanProvider struct {
	cfg    *config.Config
	log    *zap.Logger
	client *http.Client
}

// NewDoubanProvider is the constructor.
func NewDoubanProvider(cfg *config.Config, log *zap.Logger) *DoubanProvider {
	return &DoubanProvider{
		cfg:    cfg,
		log:    log,
		client: NewExternalHTTPClient(15 * time.Second),
	}
}

// Enabled reports whether Douban lookup is available. Public movie.douban.com
// suggest endpoints work without an API key; a cookie is optional and only
// helps when Douban applies stricter anti-scraping rules.
func (d *DoubanProvider) Enabled() bool {
	return true
}

// userAgents for anti-scraping randomization.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:121.0) Gecko/20100101 Firefox/121.0",
}

// DoubanMatch is the result of a Douban search hit.
type DoubanMatch struct {
	DoubanID string  `json:"douban_id"`
	Title    string  `json:"title"`
	Year     string  `json:"year"`
	Img      string  `json:"img"`
	Rating   float32 `json:"rating"`
	Type     string  `json:"type,omitempty"`
}

// Search runs a Douban subject_suggest query and returns the top match.
func (d *DoubanProvider) Search(ctx context.Context, query string) (*DoubanMatch, error) {
	if !d.Enabled() || query == "" {
		return nil, nil
	}
	u := "https://movie.douban.com/j/subject_suggest?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	d.setHeaders(req)

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("douban search: %d", resp.StatusCode)
	}

	type suggestion struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Year  string `json:"year"`
		Img   string `json:"img"`
		Type  string `json:"type"`
	}
	var results []suggestion
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	r := results[0]
	return &DoubanMatch{
		DoubanID: r.ID,
		Title:    r.Title,
		Year:     r.Year,
		Img:      r.Img,
		Type:     r.Type,
	}, nil
}

func (d *DoubanProvider) SearchMatch(ctx context.Context, query string) (*Match, error) {
	got, err := d.Search(ctx, query)
	if err != nil || got == nil {
		return nil, err
	}
	mediaType := ""
	if strings.TrimSpace(got.Type) != "" {
		mediaType = normalizeMediaType(got.Type, got.Title, "")
	}
	match := &Match{
		DoubanID:  got.DoubanID,
		MediaType: mediaType,
		Title:     got.Title,
		PosterURL: got.Img,
		Rating:    got.Rating,
	}
	if len(got.Year) >= 4 {
		_, _ = fmt.Sscanf(got.Year[:4], "%d", &match.Year)
	}
	return match, nil
}

func (d *DoubanProvider) GetMatchByID(ctx context.Context, doubanID string) (*Match, error) {
	doubanID = strings.TrimSpace(doubanID)
	if doubanID == "" {
		return nil, nil
	}
	u := "https://movie.douban.com/j/subject_abstract?subject_id=" + url.QueryEscape(doubanID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	d.setHeaders(req)
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("douban detail: %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	subject := raw
	if nested, ok := raw["subject"].(map[string]any); ok {
		subject = nested
	} else if nested, ok := raw["data"].(map[string]any); ok {
		subject = nested
	}
	title := firstStringFromMap(subject, "title", "name")
	year := 0
	if y := firstStringFromMap(subject, "year"); len(y) >= 4 {
		_, _ = fmt.Sscanf(y[:4], "%d", &year)
	}
	m := &Match{
		DoubanID:  doubanID,
		Title:     title,
		Overview:  firstStringFromMap(subject, "short_comment", "intro", "summary", "abstract"),
		PosterURL: firstStringFromMap(subject, "pic", "img", "cover", "cover_url"),
		Year:      year,
		Rating:    float32FromMap(subject, "rate", "rating"),
	}
	if m.Title == "" {
		m.Title = firstStringFromMap(raw, "title")
	}
	return m, nil
}

func (d *DoubanProvider) GetEpisodeCount(ctx context.Context, query string) (int, error) {
	match, err := d.Search(ctx, query)
	if err != nil || match == nil || strings.TrimSpace(match.DoubanID) == "" {
		return 0, err
	}
	return d.GetEpisodeCountByID(ctx, match.DoubanID)
}

func (d *DoubanProvider) GetEpisodeCountByID(ctx context.Context, doubanID string) (int, error) {
	doubanID = strings.TrimSpace(doubanID)
	if doubanID == "" {
		return 0, nil
	}
	u := "https://movie.douban.com/j/subject_abstract?subject_id=" + url.QueryEscape(doubanID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	d.setHeaders(req)

	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return 0, fmt.Errorf("douban detail: %d", resp.StatusCode)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return 0, err
	}
	for _, key := range []string{"episode_count", "episodes_count", "episodes", "eps"} {
		if count := doubanEpisodeCountFromValue(raw[key]); count > 0 {
			return count, nil
		}
	}
	for _, key := range []string{"subject", "data"} {
		if nested, ok := raw[key].(map[string]any); ok {
			for _, field := range []string{"episode_count", "episodes_count", "episodes", "eps"} {
				if count := doubanEpisodeCountFromValue(nested[field]); count > 0 {
					return count, nil
				}
			}
		}
	}
	return 0, nil
}

func (d *DoubanProvider) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgents[secureRandomIntn(len(userAgents))])
	req.Header.Set("Referer", "https://movie.douban.com/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	if cookie := strings.TrimSpace(d.cfg.Secrets.DoubanCookie); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
}
