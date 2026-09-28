// Package service — Discuz site adapter.
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

// ─── Discuz 适配器 ───────────────────────────────────────────────────────────

// DiscuzAdapter 基于 Discuz! X 的站点适配器。
type DiscuzAdapter struct {
	client *http.Client
}

// NewDiscuzAdapter 创建 Discuz 适配器。
func NewDiscuzAdapter() *DiscuzAdapter {
	return &DiscuzAdapter{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *DiscuzAdapter) Authenticate(ctx context.Context, cfg SiteConfig) error {
	u := cfg.URL + "/home.php?mod=space"
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	if status == http.StatusFound {
		return fmt.Errorf("authentication failed: redirected to login")
	}
	if status != http.StatusOK {
		return fmt.Errorf("authenticate failed: status %d", status)
	}
	body := string(data)
	if strings.Contains(body, "login") && !strings.Contains(body, "我的空间") {
		return fmt.Errorf("authentication failed: not logged in")
	}
	return nil
}

func (a *DiscuzAdapter) Search(ctx context.Context, cfg SiteConfig, keyword string, page int) (*SiteSearchResult, error) {
	return a.SearchWithCategory(ctx, cfg, keyword, "", page)
}

func (a *DiscuzAdapter) SearchWithCategory(ctx context.Context, cfg SiteConfig, keyword, category string, page int) (*SiteSearchResult, error) {
	params := url.Values{}
	params.Set("mod", "forum")
	params.Set("srchtxt", keyword)
	params.Set("searchsubmit", "true")
	if category != "" {
		params.Add("srchfid[]", category)
		params.Set("fid", category)
	}
	params.Set("page", strconv.Itoa(page))

	u := cfg.URL + "/search.php?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", status)
	}

	return parseDiscuzHTML(string(data), cfg.Name, cfg.URL)
}

func (a *DiscuzAdapter) Browse(ctx context.Context, cfg SiteConfig, category string, page int) (*SiteSearchResult, error) {
	params := url.Values{}
	if category != "" {
		params.Set("fid", category)
	}
	params.Set("page", strconv.Itoa(page))

	u := cfg.URL + "/forum.php?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("browse: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("browse failed: status %d", status)
	}

	return parseDiscuzHTML(string(data), cfg.Name, cfg.URL)
}

func (a *DiscuzAdapter) GetDetail(ctx context.Context, cfg SiteConfig, id string) (*TorrentDetail, error) {
	u := cfg.URL + "/forum.php?mod=viewthread&tid=" + id
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("detail: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("detail failed: status %d", status)
	}

	html := string(data)
	detail := &TorrentDetail{
		ID:        id,
		DetailURL: cfg.URL + "/forum.php?mod=viewthread&tid=" + id,
	}

	// Title
	titleRegex := regexp.MustCompile(`<span[^>]*id="thread_subject"[^>]*>([^<]+)</span>`)
	if m := titleRegex.FindStringSubmatch(html); len(m) >= 2 {
		detail.Title = strings.TrimSpace(m[1])
	}

	// Extract magnet/torrent links
	magnetRegex := regexp.MustCompile(`magnet:\?[^\s"'<>]+`)
	if m := magnetRegex.FindString(html); m != "" {
		detail.DownloadURL = m
	}
	torrentRegex := regexp.MustCompile(`(attachment\.php\?aid=\d+)`)
	if m := torrentRegex.FindString(html); m != "" && detail.DownloadURL == "" {
		detail.DownloadURL = cfg.URL + "/" + m
	}

	// Description
	descRegex := regexp.MustCompile(`<div[^>]*class="t_fsz"[^>]*>(.*?)</div>`)
	if m := descRegex.FindStringSubmatch(html); len(m) >= 2 {
		detail.Description = stripHTML(m[1])
	}

	return detail, nil
}

func (a *DiscuzAdapter) GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error) {
	return cfg.URL + "/forum.php?mod=viewthread&tid=" + id, nil
}

// parseDiscuzHTML 解析 Discuz HTML 响应。
func parseDiscuzHTML(html, siteName, baseURL string) (*SiteSearchResult, error) {
	result := &SiteSearchResult{
		SiteName: siteName,
		Items:    []TorrentItem{},
		Page:     1,
	}

	// Extract thread links
	threadRegex := regexp.MustCompile(`<a[^>]*href="(?:forum\.php\?mod=viewthread&tid=|thread-(\d+)-1-1)\.html"[^>]*>([^<]+)</a>`)
	matches := threadRegex.FindAllStringSubmatch(html, -1)

	for _, m := range matches {
		item := TorrentItem{}
		if m[1] != "" {
			item.ID = m[1]
		} else {
			// Extract tid from URL
			tidRegex := regexp.MustCompile(`tid=(\d+)`)
			if tidM := tidRegex.FindStringSubmatch(m[0]); len(tidM) >= 2 {
				item.ID = tidM[1]
			}
		}
		if item.ID == "" {
			continue
		}

		item.Title = strings.TrimSpace(m[2])
		item.DetailURL = baseURL + "/forum.php?mod=viewthread&tid=" + item.ID
		item.UploadTime = time.Now()

		result.Items = append(result.Items, item)
	}

	result.Total = len(result.Items)
	return result, nil
}
