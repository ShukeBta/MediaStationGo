// Package service — custom RSS site adapter.
package service

import (
	"context"
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
		ID:    id,
		Title: id,
	}, nil
}

func (a *CustomRSSAdapter) GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error) {
	return id, nil // In RSS, the ID is often the download URL
}

// parseRSSXML 解析 RSS XML 内容。
func parseRSSXML(data []byte, siteName, keyword string) (*SiteSearchResult, error) {
	result := &SiteSearchResult{
		SiteName: siteName,
		Items:    []TorrentItem{},
	}

	html := string(data)
	// Simple regex-based XML parsing for RSS items
	itemRegex := regexp.MustCompile(`<item>(.*?)</item>`)
	items := itemRegex.FindAllStringSubmatch(html, -1)

	for i, item := range items {
		ri := TorrentItem{}

		// Title
		titleRegex := regexp.MustCompile(`<title>(?:<!\[CDATA\[)?(.*?)(?:\]\]>)?</title>`)
		if m := titleRegex.FindStringSubmatch(item[1]); len(m) >= 2 {
			ri.Title = strings.TrimSpace(m[1])
		}

		// Filter by keyword
		if keyword != "" && !strings.Contains(strings.ToLower(ri.Title), strings.ToLower(keyword)) {
			continue
		}

		ri.ID = strconv.Itoa(i)

		// Link
		linkRegex := regexp.MustCompile(`<link>(?:<!\[CDATA\[)?(.*?)(?:\]\]>)?</link>`)
		if m := linkRegex.FindStringSubmatch(item[1]); len(m) >= 2 {
			ri.DetailURL = strings.TrimSpace(m[1])
			ri.DownloadURL = strings.TrimSpace(m[1])
		}

		// Description
		descRegex := regexp.MustCompile(`<description>(?:<!\[CDATA\[)?(.*?)(?:\]\]>)?</description>`)
		if m := descRegex.FindStringSubmatch(item[1]); len(m) >= 2 {
			desc := stripHTML(m[1])
			ri.Subtitle = desc
		}

		// Size from description
		sizeRegex := regexp.MustCompile(`(\d+\.?\d*)\s*(GB|MB|TB|KB)`)
		if m := sizeRegex.FindStringSubmatch(item[1]); len(m) >= 3 {
			ri.Size = parseSizeString(m[1], m[2])
		}

		// Category
		catRegex := regexp.MustCompile(`<category>(?:<!\[CDATA\[)?(.*?)(?:\]\]>)?</category>`)
		if m := catRegex.FindStringSubmatch(item[1]); len(m) >= 2 {
			ri.Category = strings.TrimSpace(m[1])
		}

		// Date
		dateRegex := regexp.MustCompile(`<pubDate>(?:<!\[CDATA\[)?(.*?)(?:\]\]>)?</pubDate>`)
		if m := dateRegex.FindStringSubmatch(item[1]); len(m) >= 2 {
			for _, layout := range []string{
				time.RFC1123, time.RFC1123Z, time.RFC3339,
				"2006-01-02 15:04:05", "2006-01-02T15:04:05-07:00",
			} {
				if t, err := time.Parse(layout, strings.TrimSpace(m[1])); err == nil {
					ri.UploadTime = t
					break
				}
			}
		}

		result.Items = append(result.Items, ri)
	}

	result.Total = len(result.Items)
	return result, nil
}

// TorrentDetail has a Description field used by RSS adapter.
// (Already defined above)
