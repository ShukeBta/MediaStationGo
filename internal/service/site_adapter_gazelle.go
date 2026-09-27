// Package service — Gazelle site adapter.
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

// ─── Gazelle 适配器 ──────────────────────────────────────────────────────────

// GazelleAdapter Gazelle 框架适配器（What.cd 开源）。
type GazelleAdapter struct {
	client *http.Client
}

// NewGazelleAdapter 创建 Gazelle 适配器。
func NewGazelleAdapter() *GazelleAdapter {
	return &GazelleAdapter{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *GazelleAdapter) Authenticate(ctx context.Context, cfg SiteConfig) error {
	u := cfg.URL + "/ajax.php?action=index"
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("authenticate failed: status %d", status)
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	if statusMsg, ok := result["status"].(string); ok && statusMsg == "failure" {
		return fmt.Errorf("authentication failed: %v", result["error"])
	}
	return nil
}

func (a *GazelleAdapter) Search(ctx context.Context, cfg SiteConfig, keyword string, page int) (*SiteSearchResult, error) {
	return a.SearchWithCategory(ctx, cfg, keyword, "", page)
}

func (a *GazelleAdapter) SearchWithCategory(ctx context.Context, cfg SiteConfig, keyword, category string, page int) (*SiteSearchResult, error) {
	params := url.Values{}
	params.Set("action", "browse")
	params.Set("searchstr", keyword)
	if category != "" {
		params.Set("filter_cat["+category+"]", "1")
	}
	params.Set("page", strconv.Itoa(page))

	u := cfg.URL + "/ajax.php?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", status)
	}

	return parseGazelleJSON(data, cfg.Name, cfg.URL)
}

func (a *GazelleAdapter) Browse(ctx context.Context, cfg SiteConfig, category string, page int) (*SiteSearchResult, error) {
	params := url.Values{}
	params.Set("action", "browse")
	if category != "" {
		params.Set("filter_cat["+category+"]", "1")
	}
	params.Set("page", strconv.Itoa(page))

	u := cfg.URL + "/ajax.php?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("browse: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("browse failed: status %d", status)
	}

	return parseGazelleJSON(data, cfg.Name, cfg.URL)
}

func (a *GazelleAdapter) GetDetail(ctx context.Context, cfg SiteConfig, id string) (*TorrentDetail, error) {
	params := url.Values{}
	params.Set("action", "torrent")
	params.Set("id", id)

	u := cfg.URL + "/ajax.php?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("detail: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("detail failed: status %d", status)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	torrent, ok := resp["torrent"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("torrent not found")
	}

	detail := &TorrentDetail{
		ID:          id,
		DetailURL:   cfg.URL + "/torrents.php?torrentid=" + id,
		DownloadURL: cfg.URL + "/torrents.php?action=download&id=" + id,
	}

	if v, ok := torrent["groupName"].(string); ok {
		detail.Title = v
	}
	if v, ok := torrent["subName"].(string); ok {
		detail.Subtitle = v
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
	if v, ok := torrent["snatched"].(float64); ok {
		detail.Snatched = int(v)
	}
	if v, ok := torrent["freeTorrent"].(string); ok && v == "1" {
		detail.Free = true
	}
	if v, ok := torrent["freeTorrent"].(bool); ok {
		detail.Free = v
	}
	if v, ok := torrent["infoHash"].(string); ok {
		detail.InfoHash = v
	}
	if v, ok := torrent["groupDesc"].(string); ok {
		detail.Description = stripHTML(v)
	}

	return detail, nil
}

func (a *GazelleAdapter) GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error) {
	return cfg.URL + "/torrents.php?action=download&id=" + id, nil
}

// parseGazelleJSON 解析 Gazelle JSON 响应。
func parseGazelleJSON(data []byte, siteName, baseURL string) (*SiteSearchResult, error) {
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	result := &SiteSearchResult{
		SiteName: siteName,
		Items:    []TorrentItem{},
	}

	if status, ok := resp["status"].(string); ok && status == "failure" {
		return result, nil
	}

	results, ok := resp["results"].([]interface{})
	if !ok {
		return result, nil
	}

	for _, r := range results {
		torrent, ok := r.(map[string]interface{})
		if !ok {
			continue
		}
		item := TorrentItem{}
		if v, ok := torrent["torrentId"].(float64); ok {
			item.ID = strconv.Itoa(int(v))
		}
		if v, ok := torrent["groupName"].(string); ok {
			item.Title = v
		}
		if v, ok := torrent["artist"].(string); ok {
			item.Subtitle = v
		}
		if v, ok := torrent["category"].(string); ok {
			item.Category = v
		}
		if v, ok := torrent["size"].(float64); ok {
			item.Size = int64(v)
		}
		if v, ok := torrent["seeders"].(float64); ok {
			item.Seeders = int(v)
		}
		if v, ok := torrent["leechers"].(float64); ok {
			item.Leechers = int(v)
		}
		if v, ok := torrent["snatched"].(float64); ok {
			item.Snatched = int(v)
		}
		if v, ok := torrent["freeTorrent"].(string); ok && v == "1" {
			item.Free = true
		}
		if v, ok := torrent["freeTorrent"].(bool); ok {
			item.Free = v
		}
		if v, ok := torrent["time"].(string); ok {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				item.UploadTime = t
			}
		}

		item.DetailURL = baseURL + "/torrents.php?torrentid=" + item.ID
		item.DownloadURL = baseURL + "/torrents.php?action=download&id=" + item.ID
		result.Items = append(result.Items, item)
	}

	if total, ok := resp["totalResults"].(float64); ok {
		result.Total = int(total)
	} else {
		result.Total = len(result.Items)
	}
	return result, nil
}
