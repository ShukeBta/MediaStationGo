// Package service — Bangumi metadata provider.
//
// Bangumi (https://bgm.tv) is the Chinese anime / manga / game database
// most users in mainland China prefer. Its public REST API is documented
// at https://bangumi.github.io/api/.
//
// We implement the minimal subset needed to enrich anime libraries:
//
//	GET /search/subject/{keywords}?type=2&responseGroup=small
//	GET /v0/subjects/{id}                                           (cover)
//
// The provider gracefully no-ops when bangumi_access_token is empty.
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

// BangumiProvider talks to https://api.bgm.tv.
type BangumiProvider struct {
	cfg    *config.Config
	log    *zap.Logger
	client *http.Client
	base   string
}

// NewBangumiProvider is the constructor.
func NewBangumiProvider(cfg *config.Config, log *zap.Logger) *BangumiProvider {
	return &BangumiProvider{
		cfg:    cfg,
		log:    log,
		base:   "https://api.bgm.tv",
		client: NewExternalHTTPClient(15 * time.Second),
	}
}

// Enabled reports whether a token is configured. Bangumi works without
// auth for read-only endpoints, but configuring a token raises the rate
// limit so we treat presence as a soft "enabled" flag.
func (b *BangumiProvider) Enabled() bool {
	// Bangumi search works without auth — keep the provider enabled
	// unconditionally, but pass the token when we have one.
	return true
}

// Search runs a Bangumi keyword search and returns the top match. Type 2
// = anime; pass 1 (book) / 6 (real) when extending later.
func (b *BangumiProvider) Search(ctx context.Context, query string) (*Match, error) {
	if query == "" {
		return nil, nil
	}
	u := fmt.Sprintf("%s/search/subject/%s?type=2&responseGroup=small",
		b.base, url.PathEscape(query))

	type subject struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		NameCN  string `json:"name_cn"`
		Summary string `json:"summary"`
		Air     string `json:"air_date"`
		Rating  struct {
			Score float32 `json:"score"`
		} `json:"rating"`
		Images struct {
			Large string `json:"large"`
		} `json:"images"`
	}
	type page struct {
		Results int       `json:"results"`
		List    []subject `json:"list"`
	}

	var p page
	if err := b.getJSON(ctx, u, &p); err != nil {
		return nil, err
	}
	if p.Results == 0 || len(p.List) == 0 {
		return nil, nil
	}
	r := p.List[0]
	title := r.NameCN
	if title == "" {
		title = r.Name
	}
	m := &Match{
		BangumiID:    r.ID,
		MediaType:    "anime",
		Title:        title,
		OriginalName: r.Name,
		Overview:     r.Summary,
		PosterURL:    normalizeBangumiImageURL(r.Images.Large),
		Rating:       r.Rating.Score,
	}
	if len(r.Air) >= 4 {
		_, _ = fmt.Sscanf(r.Air[:4], "%d", &m.Year)
	}
	return m, nil
}

func (b *BangumiProvider) GetSubject(ctx context.Context, bangumiID int) (*Match, error) {
	if bangumiID <= 0 {
		return nil, nil
	}
	u := fmt.Sprintf("%s/v0/subjects/%d", b.base, bangumiID)
	type subject struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		NameCN  string `json:"name_cn"`
		Summary string `json:"summary"`
		Air     string `json:"date"`
		Eps     int    `json:"eps"`
		Rating  struct {
			Score float32 `json:"score"`
		} `json:"rating"`
		Images struct {
			Large  string `json:"large"`
			Common string `json:"common"`
		} `json:"images"`
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}
	var r subject
	if err := b.getJSON(ctx, u, &r); err != nil {
		return nil, err
	}
	title := r.NameCN
	if title == "" {
		title = r.Name
	}
	m := &Match{
		BangumiID:    r.ID,
		MediaType:    "anime",
		Title:        title,
		OriginalName: r.Name,
		Overview:     r.Summary,
		PosterURL:    normalizeBangumiImageURL(firstText(r.Images.Large, r.Images.Common)),
		Rating:       r.Rating.Score,
	}
	if len(r.Air) >= 4 {
		_, _ = fmt.Sscanf(r.Air[:4], "%d", &m.Year)
	}
	for _, tag := range r.Tags {
		if strings.TrimSpace(tag.Name) != "" {
			m.Genres = append(m.Genres, tag.Name)
		}
	}
	return m, nil
}

func (b *BangumiProvider) GetEpisodeCount(ctx context.Context, bangumiID int) (int, error) {
	if bangumiID <= 0 {
		return 0, nil
	}
	u := fmt.Sprintf("%s/v0/subjects/%d", b.base, bangumiID)
	var subject struct {
		Eps           int `json:"eps"`
		TotalEpisodes int `json:"total_episodes"`
	}
	if err := b.getJSON(ctx, u, &subject); err != nil {
		return 0, err
	}
	if subject.Eps > 0 {
		return subject.Eps, nil
	}
	return subject.TotalEpisodes, nil
}

func (b *BangumiProvider) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "MediaStationGo/0.1 (https://github.com/ShukeBta/MediaStationGo)")
	if t := strings.TrimSpace(b.cfg.Secrets.BangumiToken); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("bangumi %s: %d", u, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func normalizeBangumiImageURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	host := strings.ToLower(u.Host)
	if host != "lain.bgm.tv" && host != "bgm.tv" && !strings.HasSuffix(host, ".bgm.tv") {
		return raw
	}
	u.Scheme = "https"
	if strings.HasPrefix(u.Path, "/pic/cover/") {
		u.Path = "/r/400" + u.Path
	}
	return u.String()
}
