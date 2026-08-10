// Package service — qBittorrent Web UI client.
//
// QBitClient is a thin wrapper around the qBittorrent /api/v2 REST API
// (https://github.com/qbittorrent/qBittorrent/wiki/WebUI-API).
//
// We only need three operations for the download flow:
//
//	POST /auth/login
//	POST /torrents/add  (multipart, accepts magnet URL or .torrent bytes)
//	GET  /torrents/info (filtered by hash)
//
// The client stores the SID cookie returned by /auth/login and reuses it
// across calls. Re-auth happens transparently on 403.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// QBitConfig holds the connection settings (typically loaded from the
// system Setting table or an env var).
type QBitConfig struct {
	BaseURL  string
	Username string
	Password string
}

// QBitTorrent is the subset of /torrents/info we surface to the API.
type QBitTorrent struct {
	Hash     string  `json:"hash"`
	ClientID string  `json:"client_id,omitempty"`
	Source   string  `json:"source,omitempty"`
	Name     string  `json:"name"`
	State    string  `json:"state"`
	Progress float32 `json:"progress"`
	DLSpeed  int64   `json:"dlspeed"`
	UpSpeed  int64   `json:"upspeed"`
	NumSeeds int     `json:"num_seeds"`
	NumLeech int     `json:"num_leechs"`
	Size     int64   `json:"size"`
	SavePath string  `json:"save_path"`
	Category string  `json:"category"`
	// ContentPath is qBittorrent's resolved payload path. For single-file
	// torrents it points at the file; for multi-file torrents it points at the
	// root folder. Prefer it for automatic organize so we do not scan the whole
	// download category.
	ContentPath string `json:"content_path"`
	// CompletionOn 是 qBittorrent 报告的完成时间（Unix 秒，未完成为 0 或负值）。
	// 用于应用重启后的「补整理」判断：只补最近完成的种子，避免每次启动
	// 都重新触发全部历史种子的整理。
	CompletionOn int64 `json:"completion_on"`
}

// QBitClient is a thread-safe qBittorrent v2 API client.
type QBitClient struct {
	log    *zap.Logger
	mu     sync.Mutex
	cfg    QBitConfig
	client *http.Client
}

var (
	qbitAddVerifyAttempts = 10
	qbitAddVerifyInterval = 800 * time.Millisecond
)

// NewQBitClient builds a fresh client. A blank URL intentionally stays blank:
// an unconfigured downloader must fail closed instead of silently trying a
// localhost qBittorrent instance.
func NewQBitClient(log *zap.Logger, cfg QBitConfig) *QBitClient {
	cfg.BaseURL = normalizeQBitBaseURL(cfg.BaseURL)
	jar, _ := cookiejar.New(nil)
	client := NewInternalHTTPClient(20 * time.Second)
	client.Jar = jar
	return &QBitClient{
		log:    log,
		cfg:    cfg,
		client: client,
	}
}

// Configure rotates the client to a new endpoint and re-auths next call.
func (q *QBitClient) Configure(cfg QBitConfig) {
	q.mu.Lock()
	defer q.mu.Unlock()
	cfg.BaseURL = normalizeQBitBaseURL(cfg.BaseURL)
	q.cfg = cfg
	jar, _ := cookiejar.New(nil)
	q.client.Jar = jar
}

func (q *QBitClient) IsConfigured() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return strings.TrimSpace(q.cfg.BaseURL) != ""
}

func normalizeQBitBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	normalized, err := normalizeDownloadClientEndpoint("qbittorrent", raw)
	if err != nil {
		return strings.TrimRight(raw, "/")
	}
	return normalized
}

// Login performs POST /api/v2/auth/login.
func (q *QBitClient) Login(ctx context.Context) error {
	if q.cfg.BaseURL == "" {
		return errors.New("qbittorrent base url not configured")
	}
	return qbitLogin(ctx, q.client, q.cfg.BaseURL, q.cfg.Username, q.cfg.Password)
}

// List returns every torrent (optionally filtered by status: all / downloading / completed).
func (q *QBitClient) List(ctx context.Context, filter string) ([]QBitTorrent, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return nil, err
	}
	return q.listLocked(ctx, filter)
}

func (q *QBitClient) listLocked(ctx context.Context, filter string) ([]QBitTorrent, error) {
	u := strings.TrimRight(q.cfg.BaseURL, "/") + "/api/v2/torrents/info"
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if filter != "" {
		query := req.URL.Query()
		query.Set("filter", filter)
		req.URL.RawQuery = query.Encode()
	}
	req.Header.Set("Referer", q.cfg.BaseURL)
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("qbittorrent list: %d", resp.StatusCode)
	}
	var out []QBitTorrent
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

// Delete removes a torrent (optionally with its files).
func (q *QBitClient) Delete(ctx context.Context, hash string, deleteFiles bool) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}
	form := url.Values{}
	form.Set("hashes", hash)
	if deleteFiles {
		form.Set("deleteFiles", "true")
	} else {
		form.Set("deleteFiles", "false")
	}
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/delete",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.cfg.BaseURL)
	req.Header.Set("Origin", q.cfg.BaseURL)
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("qbittorrent delete: %d", resp.StatusCode)
	}
	return nil
}

func (q *QBitClient) Pause(ctx context.Context, hash string) error {
	return q.torrentAction(ctx, hash, "pause", "stop")
}

func (q *QBitClient) Resume(ctx context.Context, hash string) error {
	return q.torrentAction(ctx, hash, "resume", "start")
}

func (q *QBitClient) torrentAction(ctx context.Context, hash string, actions ...string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}
	baseURL := strings.TrimRight(q.cfg.BaseURL, "/")
	form := url.Values{"hashes": []string{hash}}
	var lastErr error
	for _, action := range actions {
		req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
			baseURL+"/api/v2/torrents/"+action, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", baseURL)
		req.Header.Set("Origin", baseURL)
		resp, err := q.client.Do(req)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode < 400 {
			return nil
		}
		lastErr = fmt.Errorf("qbittorrent %s: %d: %s", action, resp.StatusCode, strings.TrimSpace(string(body)))
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusMethodNotAllowed {
			break
		}
	}
	return lastErr
}

// SetLocation moves a torrent's data to a new save directory via
// POST /api/v2/torrents/setLocation. qBittorrent performs the physical move
// itself and keeps seeding from the new location — this is the seeding-safe
// way to relocate downloaded PT files. location must be an absolute path the
// qBittorrent process can write to.
func (q *QBitClient) SetLocation(ctx context.Context, hash, location string) error {
	if strings.TrimSpace(hash) == "" {
		return errors.New("qbittorrent setLocation: empty hash")
	}
	if strings.TrimSpace(location) == "" {
		return errors.New("qbittorrent setLocation: empty location")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}
	form := url.Values{}
	form.Set("hashes", hash)
	form.Set("location", location)
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/setLocation",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.cfg.BaseURL)
	req.Header.Set("Origin", q.cfg.BaseURL)
	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusBadRequest {
		return errors.New("qbittorrent setLocation: 保存路径无效")
	}
	if resp.StatusCode == http.StatusConflict {
		return errors.New("qbittorrent setLocation: 无法写入目标路径 (权限或磁盘问题)")
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("qbittorrent setLocation: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	q.log.Info("qbittorrent: torrent relocated", zap.String("hash", hash), zap.String("location", location))
	return nil
}

// ensureAuth makes sure we have a valid SID cookie. Cheap on the happy
// path; logs in transparently otherwise.
func (q *QBitClient) ensureAuth(ctx context.Context) error {
	if strings.TrimSpace(q.cfg.BaseURL) == "" {
		return errors.New("qbittorrent base url not configured")
	}
	u, err := url.Parse(q.cfg.BaseURL)
	if err != nil {
		return err
	}
	if cookies := q.client.Jar.Cookies(u); len(cookies) > 0 {
		for _, c := range cookies {
			if strings.EqualFold(c.Name, "SID") && c.Value != "" {
				return nil
			}
		}
	}
	return q.Login(ctx)
}
