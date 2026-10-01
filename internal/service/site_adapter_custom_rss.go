// Package service — custom RSS site adapter.
package service

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ─── Custom RSS 适配器 ───────────────────────────────────────────────────────

// CustomRSSAdapter 自定义 RSS 源适配器。
type CustomRSSAdapter struct {
	client *http.Client
}

// NewCustomRSSAdapter 创建 Custom RSS 适配器。
func NewCustomRSSAdapter() *CustomRSSAdapter {
	return &CustomRSSAdapter{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *CustomRSSAdapter) Authenticate(ctx context.Context, cfg SiteConfig) error {
	// RSS 源通常不需要认证，或者认证通过 URL 参数
	if cfg.URL == "" {
		return fmt.Errorf("RSS URL is required")
	}
	_, status, err := doRequest(ctx, a.client, "GET", cfg.URL, cfg, nil)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("authenticate failed: status %d", status)
	}
	return nil
}

func (a *CustomRSSAdapter) Search(ctx context.Context, cfg SiteConfig, keyword string, page int) (*SiteSearchResult, error) {
	searchURL := cfg.URL
	// If extra has search URL template, use it
	if searchTpl, ok := cfg.Extra["search_url"]; ok && searchTpl != "" {
		searchURL = strings.ReplaceAll(searchTpl, "{keyword}", url.QueryEscape(keyword))
		searchURL = strings.ReplaceAll(searchURL, "{page}", strconv.Itoa(page))
	}

	data, status, err := doRequest(ctx, a.client, "GET", searchURL, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", status)
	}

	result, err := parseRSSXML(data, cfg.Name, keyword)
	if err != nil {
		return nil, fmt.Errorf("parse RSS: %w", err)
	}

	if page > 1 {
		// Simple pagination for RSS: skip items already seen
		start := (page - 1) * 50
		if start < len(result.Items) {
			result.Items = result.Items[start:]
		} else {
			result.Items = []TorrentItem{}
		}
	}
	result.Page = page

	return result, nil
}

func (a *CustomRSSAdapter) SearchWithCategory(ctx context.Context, cfg SiteConfig, keyword, category string, page int) (*SiteSearchResult, error) {
	return a.Search(ctx, cfg, keyword, page)
}

func (a *CustomRSSAdapter) Browse(ctx context.Context, cfg SiteConfig, category string, page int) (*SiteSearchResult, error) {
	// RSS browse is essentially the same as search with empty keyword
	return a.Search(ctx, cfg, "", page)
}

func (a *CustomRSSAdapter) GetDetail(ctx context.Context, cfg SiteConfig, id string) (*TorrentDetail, error) {
	// RSS typically doesn't support detail page; return basic info
	return &TorrentDetail{
		ID: id,
	}, nil
}

func (a *CustomRSSAdapter) GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error) {
	id = strings.TrimSpace(id)
	parsed, err := url.Parse(id)
	if err == nil && ((parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" || parsed.Scheme == "magnet" && parsed.Query().Get("xt") != "") {
		return id, nil
	}
	return "", fmt.Errorf("RSS resource has no valid download URL; refresh the resource list")
}

// parseRSSXML 解析 RSS XML 内容。
func parseRSSXML(data []byte, siteName, keyword string) (*SiteSearchResult, error) {
	result := &SiteSearchResult{
		SiteName: siteName,
		Items:    []TorrentItem{},
	}

	var feed struct {
		XMLName xml.Name `xml:"rss"`
		Items   []struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Category    string `xml:"category"`
			PubDate     string `xml:"pubDate"`
			Enclosure   struct {
				URL    string `xml:"url,attr"`
				Length int64  `xml:"length,attr"`
			} `xml:"enclosure"`
		} `xml:"channel>item"`
	}
	if err := xml.Unmarshal(data, &feed); err != nil {
		return nil, err
	}
	sizeRegex := regexp.MustCompile(`(?i)(\d+\.?\d*)\s*(GB|MB|TB|KB)`)
	for _, item := range feed.Items {
		ri := TorrentItem{
			Title:       strings.TrimSpace(item.Title),
			DetailURL:   strings.TrimSpace(item.Link),
			DownloadURL: strings.TrimSpace(item.Enclosure.URL),
			Subtitle:    stripHTML(item.Description),
			Category:    strings.TrimSpace(item.Category),
			Size:        item.Enclosure.Length,
		}
		if ri.Title == "" || keyword != "" && !strings.Contains(strings.ToLower(ri.Title), strings.ToLower(keyword)) {
			continue
		}
		if ri.DownloadURL == "" {
			ri.DownloadURL = ri.DetailURL
		}
		// Feed ordering changes as torrents are added. A download URL is both
		// a stable resource identity and what this adapter resolves for download.
		ri.ID = ri.DownloadURL
		if ri.Size <= 0 {
			if m := sizeRegex.FindStringSubmatch(ri.Subtitle); len(m) >= 3 {
				ri.Size = parseSizeString(m[1], strings.ToUpper(m[2]))
			}
		}
		for _, layout := range []string{time.RFC1123, time.RFC1123Z, time.RFC3339, "2006-01-02 15:04:05"} {
			if published, err := time.Parse(layout, strings.TrimSpace(item.PubDate)); err == nil {
				ri.UploadTime = published
				break
			}
		}
		result.Items = append(result.Items, ri)
	}

	result.Total = len(result.Items)
	return result, nil
}
