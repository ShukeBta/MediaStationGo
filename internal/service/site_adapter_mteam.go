// Package service — M-Team site adapter.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ─── MTeam 适配器 ────────────────────────────────────────────────────────────

// MTeamAdapter MTeam.cc 独立站适配器。
type MTeamAdapter struct {
	client *http.Client
}

// NewMTeamAdapter 创建 MTeam 适配器。
func NewMTeamAdapter() *MTeamAdapter {
	return &MTeamAdapter{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (a *MTeamAdapter) Authenticate(ctx context.Context, cfg SiteConfig) error {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return fmt.Errorf("M-Team 需要填写 API Access Token（控制台 → 实验室 → 存取令牌），不能使用 Cookie 访问开放 API")
	}
	// 与旧版参考实现对齐：
	// 用 camelCase 参数（pageNumber / pageSize），同时接受 code 为字符串 "0"
	// 或数值 0；兼容 M-Team v3 API 不同版本的返回。
	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointSearch); err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	u := cfg.URL + "/api/torrent/search"
	payload := `{"pageNumber":1,"pageSize":1,"mode":"all"}`
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, []byte(payload))
	if err != nil {
		return mteamRequestError("authenticate", cfg, err)
	}
	preview := string(data)
	if len(preview) > 400 {
		preview = preview[:400] + "..."
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return fmt.Errorf("authentication failed: status %d, body=%s", status, preview)
	}
	if status >= 300 && status < 400 {
		return fmt.Errorf("authentication failed: HTTP %d (API Key 无效或未登录), body=%s", status, preview)
	}
	if status != http.StatusOK {
		return fmt.Errorf("authenticate failed: status %d, body=%s", status, preview)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse response: %w (body=%s)", err, preview)
	}
	if mteamCodeOK(resp["code"]) {
		return nil
	}
	msg, _ := resp["message"].(string)
	if msg == "" {
		msg = fmt.Sprintf("code=%s", mteamCodeString(resp["code"]))
	}
	return fmt.Errorf("authentication failed: %s (body=%s)", msg, preview)
}

func (a *MTeamAdapter) Search(ctx context.Context, cfg SiteConfig, keyword string, page int) (*SiteSearchResult, error) {
	// 与参考项目对齐：使用 camelCase 字段名，page 从 1 开始。
	if page <= 0 {
		page = 1
	}
	payload := map[string]interface{}{
		"keyword":    keyword,
		"pageNumber": page,
		"pageSize":   50,
	}
	body, _ := json.Marshal(payload)

	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointSearch); err != nil {
		return nil, err
	}
	u := cfg.URL + "/api/torrent/search"
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, body)
	if err != nil {
		return nil, mteamRequestError("search", cfg, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("search failed: status %d", status)
	}

	return parseMTeamJSON(data, cfg.Name, cfg.URL)
}

func (a *MTeamAdapter) Browse(ctx context.Context, cfg SiteConfig, category string, page int) (*SiteSearchResult, error) {
	if page <= 0 {
		page = 1
	}
	payload := map[string]interface{}{
		"keyword":    "",
		"pageNumber": page,
		"pageSize":   50,
	}
	if category != "" {
		payload["categories"] = []string{category}
	}
	body, _ := json.Marshal(payload)

	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointSearch); err != nil {
		return nil, err
	}
	u := cfg.URL + "/api/torrent/search"
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, body)
	if err != nil {
		return nil, mteamRequestError("browse", cfg, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("browse failed: status %d", status)
	}

	return parseMTeamJSON(data, cfg.Name, cfg.URL)
}

func (a *MTeamAdapter) GetDetail(ctx context.Context, cfg SiteConfig, id string) (*TorrentDetail, error) {
	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointDetail); err != nil {
		return nil, err
	}
	u := cfg.URL + "/api/torrent/detail?id=" + url.QueryEscape(id)
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, nil)
	if err != nil {
		return nil, mteamRequestError("detail", cfg, err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("detail failed: status %d", status)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	dataField, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("detail not found")
	}

	detail := &TorrentDetail{
		ID:        id,
		DetailURL: cfg.URL + "/detail/" + id,
	}

	if v, ok := dataField["name"].(string); ok {
		detail.Title = v
	}
	if v, ok := dataField["subtitle"].(string); ok {
		detail.Subtitle = v
	}
	if v, ok := dataField["size"].(float64); ok {
		detail.Size = int64(v)
	}
	if v, ok := dataField["status"].(map[string]interface{}); ok {
		if seeders, ok := v["seeders"].(float64); ok {
			detail.Seeders = int(seeders)
		}
		if leechers, ok := v["leechers"].(float64); ok {
			detail.Leechers = int(leechers)
		}
		if snatched, ok := v["completed"].(float64); ok {
			detail.Snatched = int(snatched)
		}
	}
	if v, ok := dataField["free"].(bool); ok {
		detail.Free = v
	}
	if v, ok := dataField["download"].(string); ok {
		detail.DownloadURL = v
	}
	if v, ok := dataField["description"].(string); ok {
		detail.Description = stripHTML(v)
	}

	return detail, nil
}

// GetDownloadURL 解析 M-Team 种子的真实下载链接。
//
// M-Team v3 流程：
//
//	POST /api/torrent/genDlToken?id={tid}     (带 x-api-key)
//	→ {"code":"0","data":"https://api.m-team.cc/api/rss/dlv2?sign=..."}
//
// 拿到的 sign URL 可被任何下载客户端无认证地直接 GET。这是旧版参考实现
// _download_torrent_file 方法的子集。
func (a *MTeamAdapter) GetDownloadURL(ctx context.Context, cfg SiteConfig, id string) (string, error) {
	if err := reserveMTeamAPIQuota(ctx, cfg, mteamAPIEndpointDownload); err != nil {
		return "", err
	}
	u := cfg.URL + "/api/torrent/genDlToken?id=" + id
	// genDlToken 是 POST 但参数走 query string；body 留空。
	data, status, err := doRequestJSON(ctx, a.client, "POST", u, cfg, []byte("{}"))
	if err != nil {
		return "", mteamRequestError("genDlToken", cfg, err)
	}
	if status >= 300 {
		return "", fmt.Errorf("genDlToken: HTTP %d", status)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("genDlToken parse: %w", err)
	}
	codeStr := ""
	switch v := resp["code"].(type) {
	case string:
		codeStr = v
	case float64:
		codeStr = strconv.Itoa(int(v))
	}
	if codeStr != "0" && codeStr != "200" {
		msg, _ := resp["message"].(string)
		if msg == "" {
			msg = "unknown error"
		}
		return "", fmt.Errorf("genDlToken: %s", msg)
	}
	dl, _ := resp["data"].(string)
	if dl == "" {
		return "", fmt.Errorf("genDlToken: empty data field")
	}
	return dl, nil
}

func mteamRequestError(action string, cfg SiteConfig, err error) error {
	if err == nil {
		return nil
	}
	if isSiteRequestTimeout(err) {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		return fmt.Errorf("%s: M-Team API request timed out after %s; check Docker/IPv6/proxy access to api.m-team.cc or increase the site timeout to 45-60s: %w",
			action, timeout.Round(time.Second), err)
	}
	return fmt.Errorf("%s: %w", action, err)
}

func isSiteRequestTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var timeout interface{ Timeout() bool }
	return errors.As(err, &timeout) && timeout.Timeout()
}
