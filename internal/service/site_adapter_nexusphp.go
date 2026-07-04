// Package service — NexusPHP site adapter.
package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ─── NexusPHP 适配器 ─────────────────────────────────────────────────────────

// NexusPHPAdapter NexusPHP 框架适配器（馒头、HDHome、CHDBits 等）。
type NexusPHPAdapter struct {
	client *http.Client
}

// NewNexusPHPAdapter 创建 NexusPHP 适配器。
func NewNexusPHPAdapter() *NexusPHPAdapter {
	return &NexusPHPAdapter{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *NexusPHPAdapter) Authenticate(ctx context.Context, cfg SiteConfig) error {
	// 走 doRequest 以便复用代理 / FlareSolverr / 浏览器头。
	data, status, err := doRequest(ctx, a.client, "GET", cfg.URL+"/index.php", cfg, nil)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}

	if status == http.StatusFound {
		return fmt.Errorf("authentication failed: redirected to login page")
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("authentication failed: status %d", status)
	}
	if status >= 400 {
		return fmt.Errorf("authentication failed: status %d", status)
	}

	body := string(data)
	// NexusPHP 登录后页面通常包含 logout 或 userdetails；
	// 仅当二者都不存在且明确显示登录表单时才判失败。
	if strings.Contains(body, "userdetails") || strings.Contains(body, "logout") || strings.Contains(body, "退出") {
		return nil
	}
	if strings.Contains(body, "takelogin.php") || strings.Contains(body, "id=\"loginform\"") {
		return fmt.Errorf("authentication failed: not logged in")
	}
	// 状态码 OK 但页面不含明显标记时不再武断判失败。
	return nil
}

func (a *NexusPHPAdapter) Search(ctx context.Context, cfg SiteConfig, keyword string, page int) (*SiteSearchResult, error) {
	params := url.Values{}
	params.Set("searchstr", keyword)
	params.Set("search", keyword)
	params.Set("search_area", "0")
	params.Set("search_mode", "0")
	params.Set("page", strconv.Itoa(page))
	params.Set("inclbookmarked", "0")
	params.Set("incldead", "0")

	u := cfg.URL + "/torrents.php?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("search request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", status)
	}

	body := string(data)
	if nexusPHPPageLooksLogin(body) {
		return nil, fmt.Errorf("search failed: not logged in or cookie expired")
	}
	return parseNexusPHPHTML(body, cfg.Name, cfg.URL)
}

func (a *NexusPHPAdapter) Browse(ctx context.Context, cfg SiteConfig, category string, page int) (*SiteSearchResult, error) {
	params := url.Values{}
	if category != "" {
		params.Set("cat", category)
	}
	params.Set("page", strconv.Itoa(page))

	u := cfg.URL + "/torrents.php?" + params.Encode()
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("browse request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("browse failed: status %d", status)
	}

	body := string(data)
	if nexusPHPPageLooksLogin(body) {
		return nil, fmt.Errorf("browse failed: not logged in or cookie expired")
	}
	return parseNexusPHPHTML(body, cfg.Name, cfg.URL)
}

func (a *NexusPHPAdapter) GetDetail(ctx context.Context, cfg SiteConfig, id string) (*TorrentDetail, error) {
	u := cfg.URL + "/details.php?id=" + id
	data, status, err := doRequest(ctx, a.client, "GET", u, cfg, nil)
	if err != nil {
		return nil, fmt.Errorf("detail request: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("detail failed: status %d", status)
	}

	return parseNexusPHPDetailHTML(string(data), id, cfg.URL)
}

func (a *NexusPHPAdapter) GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error) {
	return cfg.URL + "/download.php?id=" + id, nil
}
