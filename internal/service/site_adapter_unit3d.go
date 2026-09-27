// Package service — UNIT3D site adapter.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// ─── UNIT3D 适配器 ───────────────────────────────────────────────────────────

// UNIT3DAdapter UNIT3D 框架适配器。
type UNIT3DAdapter struct {
	client *http.Client
}

// NewUNIT3DAdapter 创建 UNIT3D 适配器。
func NewUNIT3DAdapter() *UNIT3DAdapter {
	return &UNIT3DAdapter{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *UNIT3DAdapter) Authenticate(ctx context.Context, cfg SiteConfig) error {
	u := cfg.URL + "/api/torrents?limit=1"
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("authentication failed: status %d", status)
	}
	if status != http.StatusOK {
		return fmt.Errorf("authenticate failed: status %d", status)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err == nil {
		if errMsg, ok := resp["message"].(string); ok {
			return fmt.Errorf("authentication failed: %s", errMsg)
		}
	}
	return nil
}

func (a *UNIT3DAdapter) Search(ctx context.Context, cfg SiteConfig, keyword string, page int) (*SiteSearchResult, error) {
	return a.SearchWithCategory(ctx, cfg, keyword, "", page)
}

func (a *UNIT3DAdapter) SearchWithCategory(ctx context.Context, cfg SiteConfig, keyword, category string, page int) (*SiteSearchResult, error) {
	params := url.Values{}
	params.Set("search", keyword)
	if category != "" {
		params.Set("category", category)
	}
	params.Set("page", strconv.Itoa(page))

	u := cfg.URL + "/api/torrents?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", status)
	}

	return parseUNIT3DJSON(data, cfg.Name, cfg.URL)
}

func (a *UNIT3DAdapter) Browse(ctx context.Context, cfg SiteConfig, category string, page int) (*SiteSearchResult, error) {
	params := url.Values{}
	if category != "" {
		params.Set("category", category)
	}
	params.Set("page", strconv.Itoa(page))

	u := cfg.URL + "/api/torrents?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("browse: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("browse failed: status %d", status)
	}

	return parseUNIT3DJSON(data, cfg.Name, cfg.URL)
}

func (a *UNIT3DAdapter) Categories(ctx context.Context, cfg SiteConfig) ([]SiteCategory, error) {
	data, status, err := doRequest(ctx, a.client, "GET", cfg.URL+"/api/categories", cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("categories: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("categories failed: status %d", status)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse categories: %w", err)
	}
	payload := any(raw)
	if dataField, ok := raw["data"]; ok && dataField != nil {
		payload = dataField
	}
	return dedupeSiteCategories(collectSiteCategoriesFromJSON(payload, cfg.Type, "")), nil
}

func (a *UNIT3DAdapter) GetDetail(ctx context.Context, cfg SiteConfig, id string) (*TorrentDetail, error) {
	u := cfg.URL + "/api/torrents/" + id
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("detail: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("detail failed: status %d", status)
	}

	var torrent map[string]interface{}
	if err := json.Unmarshal(data, &torrent); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	detail := &TorrentDetail{
		ID:        id,
		DetailURL: cfg.URL + "/torrents/" + id,
	}

	if v, ok := torrent["name"].(string); ok {
		detail.Title = v
	}
	if v, ok := torrent["description"].(string); ok {
		detail.PosterURL = firstImageURLFromHTML(cfg.URL, v)
		detail.Description = stripHTML(v)
	}
	if v, ok := torrent["size"].(float64); ok {
		detail.Size = int64(v)
	}
	if v, ok := torrent["seeders"].(float64); ok {
		detail.Seeders = int(v)
	}
	if v, ok := torrent["leechers"].(float64); ok {
		detail.Leechers = int(v)
	}
	if v, ok := torrent["times_completed"].(float64); ok {
		detail.Snatched = int(v)
	}
	if v, ok := torrent["free"].(bool); ok {
		detail.Free = v
	}
	if v, ok := torrent["info_hash"].(string); ok {
		detail.InfoHash = v
	}

	detail.DownloadURL = cfg.URL + "/api/torrents/" + id + "/download"
	return detail, nil
}

func (a *UNIT3DAdapter) GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error) {
	return cfg.URL + "/api/torrents/" + id + "/download", nil
}

// parseUNIT3DJSON 解析 UNIT3D JSON 响应。
func parseUNIT3DJSON(data []byte, siteName, baseURL string) (*SiteSearchResult, error) {
	var resp struct {
		Data []map[string]interface{} `json:"data"`
		Meta struct {
			Total       int `json:"total"`
			CurrentPage int `json:"current_page"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	result := &SiteSearchResult{
		SiteName: siteName,
		Items:    []TorrentItem{},
		Page:     resp.Meta.CurrentPage,
		Total:    resp.Meta.Total,
	}

	for _, t := range resp.Data {
		item := TorrentItem{}
		if v, ok := t["id"].(float64); ok {
			item.ID = strconv.Itoa(int(v))
		}
		if v, ok := t["name"].(string); ok {
			item.Title = v
		}
		if v, ok := t["poster"].(string); ok {
			item.PosterURL = absolutizeURL(baseURL, v)
		} else if v, ok := t["cover"].(string); ok {
			item.PosterURL = absolutizeURL(baseURL, v)
		}
		if v, ok := t["backdrop"].(string); ok {
			item.BackdropURL = absolutizeURL(baseURL, v)
		}
		if v, ok := t["category"].(map[string]interface{}); ok {
			if name, ok := v["name"].(string); ok {
				item.Category = name
			}
		}
		if v, ok := t["size"].(float64); ok {
			item.Size = int64(v)
		}
		if v, ok := t["seeders"].(float64); ok {
			item.Seeders = int(v)
		}
		if v, ok := t["leechers"].(float64); ok {
			item.Leechers = int(v)
		}
		if v, ok := t["times_completed"].(float64); ok {
			item.Snatched = int(v)
		}
		if v, ok := t["free"].(bool); ok {
			item.Free = v
		}
		if v, ok := t["created_at"].(string); ok {
			if t2, err := time.Parse(time.RFC3339, v); err == nil {
				item.UploadTime = t2
			}
		}

		item.DetailURL = baseURL + "/torrents/" + item.ID
		item.DownloadURL = baseURL + "/api/torrents/" + item.ID + "/download"
		result.Items = append(result.Items, item)
	}

	return result, nil
}
