// Package service — PT 站点适配器接口及 6 种适配器实现。
package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/helper"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SiteConfig 站点配置（从 model.Site 解密后的纯文本）。
type SiteConfig struct {
	SiteID          string
	Name            string
	Type            string
	URL             string
	AuthType        string
	Cookie          string
	APIKey          string
	AuthHeader      string
	UserAgent       string            // 自定义 User-Agent
	Timeout         time.Duration     // 请求超时
	Extra           map[string]string // JSON 扩展配置
	FlareSolverrURL string            // FlareSolverr 服务地址（用于浏览器模拟绕过 Cloudflare/WAF）
	UseProxy        bool              // 通过 HTTP(S)_PROXY 环境变量出站
	RateLimit       bool
	rateLimiter     siteAPIRateLimiter
}

// SiteSearchResult 站点搜索结果（按站点分组的批量搜索结果）。
type SiteSearchResult struct {
	SiteName string        `json:"site_name"`
	Items    []TorrentItem `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
}

// TorrentItem 种子条目。
type TorrentItem struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	PosterURL   string     `json:"poster_url,omitempty"`
	BackdropURL string     `json:"backdrop_url,omitempty"`
	Subtitle    string     `json:"subtitle"`
	Labels      string     `json:"labels,omitempty"`
	Category    string     `json:"category"`
	Size        int64      `json:"size"`
	Seeders     int        `json:"seeders"`
	Leechers    int        `json:"leechers"`
	Snatched    int        `json:"snatched"`
	Free        bool       `json:"free"`
	FreeEndAt   *time.Time `json:"free_end_at"`
	UploadTime  time.Time  `json:"upload_time"`
	DetailURL   string     `json:"detail_url"`
	DownloadURL string     `json:"download_url"`
}

// TorrentDetail 种子详情。
type TorrentDetail struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	PosterURL   string     `json:"poster_url,omitempty"`
	BackdropURL string     `json:"backdrop_url,omitempty"`
	Subtitle    string     `json:"subtitle"`
	Category    string     `json:"category"`
	Size        int64      `json:"size"`
	Seeders     int        `json:"seeders"`
	Leechers    int        `json:"leechers"`
	Snatched    int        `json:"snatched"`
	Free        bool       `json:"free"`
	FreeEndAt   *time.Time `json:"free_end_at"`
	UploadTime  time.Time  `json:"upload_time"`
	DetailURL   string     `json:"detail_url"`
	DownloadURL string     `json:"download_url"`
	InfoHash    string     `json:"info_hash,omitempty"`
	ImdbID      string     `json:"imdb_id,omitempty"`
	TMDbID      string     `json:"tmdb_id,omitempty"`
	DoubanID    string     `json:"douban_id,omitempty"`
	Description string     `json:"description,omitempty"`
	Genres      []string   `json:"genres,omitempty"`
	Tags        []string   `json:"tags,omitempty"`
	Images      []string   `json:"images,omitempty"`
	Files       []string   `json:"files,omitempty"`
}

// SiteAdapter 站点适配器接口。
type SiteAdapter interface {
	// Authenticate 测试站点认证是否有效。
	Authenticate(ctx context.Context, cfg SiteConfig) error

	// Search 搜索种子。
	Search(ctx context.Context, cfg SiteConfig, keyword string, page int) (*SiteSearchResult, error)

	// Browse 浏览种子列表。
	Browse(ctx context.Context, cfg SiteConfig, category string, page int) (*SiteSearchResult, error)

	// GetDetail 获取种子详情。
	GetDetail(ctx context.Context, cfg SiteConfig, id string) (*TorrentDetail, error)

	// GetDownloadURL 获取下载链接。
	GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error)
}

// newHTTPClient 创建带有认证头的 HTTP 客户端。
// 当 cfg.UseProxy 为 true 时，会读取 HTTP(S)_PROXY 环境变量；
// 否则忽略环境变量直连。
func newHTTPClient(cfg SiteConfig, timeout time.Duration) *http.Client {
	secs := int(timeout.Seconds())
	if secs <= 0 {
		secs = 30
	}
	return helper.NewSiteHTTPClient(secs, cfg.UseProxy)
}

func siteRequestHTTPClient(client *http.Client, cfg SiteConfig) *http.Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	if client == nil || cfg.UseProxy || client.Timeout != timeout {
		return newHTTPClient(cfg, timeout)
	}
	return client
}

// buildRequest 构建带认证的 HTTP 请求。
func buildRequest(ctx context.Context, method, rawURL string, cfg SiteConfig, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}

	switch cfg.AuthType {
	case "cookie":
		if cfg.Cookie != "" {
			req.Header.Set("Cookie", cfg.Cookie)
		}
	case "api_key":
		if cfg.APIKey != "" {
			if isYemaPTConfig(cfg) {
				req.Header.Set("Authorization", cfg.APIKey)
			} else {
				// M-Team / UNIT3D 等开放 API 的 PT 站点使用 `x-api-key`。
				req.Header.Set("x-api-key", cfg.APIKey)
			}
		}
	case "auth_header":
		if cfg.AuthHeader != "" {
			parts := strings.SplitN(cfg.AuthHeader, ":", 2)
			if len(parts) == 2 {
				req.Header.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
			} else {
				req.Header.Set("Authorization", "Bearer "+cfg.AuthHeader)
			}
		}
	}

	// 使用 SiteConfig 中的 UserAgent（如果提供），否则使用默认值
	userAgent := cfg.UserAgent
	if userAgent == "" {
		userAgent = model.DefaultUserAgent
	}
	req.Header.Set("User-Agent", userAgent)
	return req, nil
}

// doRequest 执行 HTTP 请求并返回响应体。
// 当 cfg.FlareSolverrURL 已配置且方法为 GET 时，通过 FlareSolverr 代理请求
// 以绕过 Cloudflare/WAF 挑战验证。
func doRequest(ctx context.Context, client *http.Client, method, rawURL string, cfg SiteConfig, body io.Reader) ([]byte, int, error) {
	// ── FlareSolverr 浏览器模拟路径（仅 GET） ──────────────────────────
	if cfg.FlareSolverrURL != "" && method == "GET" {
		timeout := int(cfg.Timeout.Seconds())
		if timeout <= 0 {
			timeout = 30
		}
		pageBody, err := helper.FetchURLWithFlareSolverr(
			cfg.FlareSolverrURL, rawURL, cfg.Cookie, timeout, "", nil)
		if err != nil {
			return nil, 0, fmt.Errorf("flareSolverr: %w", err)
		}
		return []byte(pageBody), http.StatusOK, nil
	}

	// ── 直接 HTTP 请求路径 ─────────────────────────────────────────────
	req, err := buildRequest(ctx, method, rawURL, cfg, body)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	// 当站点开启了「使用代理」开关时，使用本次请求专用的、读取 HTTP(S)_PROXY
	// 的 client；否则沿用适配器持有的全局 client。这与前端勾选行为对齐。
	httpClient := siteRequestHTTPClient(client, cfg)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}

// ─── 辅助函数 ────────────────────────────────────────────────────────────────

// doRequestJSON 执行 JSON 请求。
func doRequestJSON(ctx context.Context, client *http.Client, method, rawURL string, cfg SiteConfig, body []byte) ([]byte, int, error) {
	req, err := buildRequest(ctx, method, rawURL, cfg, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	if body != nil {
		req.Body = io.NopCloser(strings.NewReader(string(body)))
		req.ContentLength = int64(len(body))
	}

	httpClient := siteRequestHTTPClient(client, cfg)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return data, resp.StatusCode, nil
}
