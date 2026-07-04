// Package service — qBittorrent 下载适配器。
//
// QBitAdapter 实现了 DownloadAdapter 接口，通过 qBittorrent WebUI API
// 管理下载任务。底层使用与 QBitClient 相同的 HTTP API 调用逻辑。
package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"sync"
	"time"
)

// QBitAdapter 是 qBittorrent 的 DownloadAdapter 实现。
type QBitAdapter struct {
	mu       sync.Mutex
	cfg      DownloadClientConfig
	client   *http.Client
	LoggedIn bool
}

// NewQBitAdapter 创建新的 qBittorrent 适配器。
func NewQBitAdapter() *QBitAdapter {
	jar, _ := cookiejar.New(nil)
	client := NewInternalHTTPClient(20 * time.Second)
	client.Jar = jar
	return &QBitAdapter{
		client: client,
	}
}

// Initialize 配置并初始化 qBittorrent 连接。
func (a *QBitAdapter) Initialize(ctx context.Context, cfg DownloadClientConfig) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	endpoint, err := normalizeDownloadClientEndpoint("qbittorrent", cfg.Host)
	if err != nil {
		return err
	}
	cfg.Host = endpoint
	a.cfg = cfg
	a.LoggedIn = false
	jar, _ := cookiejar.New(nil)
	a.client.Jar = jar
	return a.loginLocked(ctx)
}

// Ping 测试连接。
func (a *QBitAdapter) Ping(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loginLocked(ctx)
}

// AddTorrent 通过 URL 添加种子。
func (a *QBitAdapter) AddTorrent(ctx context.Context, torrentURL, savePath string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureAuthLocked(ctx); err != nil {
		return "", err
	}

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("urls", torrentURL)
	if savePath != "" {
		_ = w.WriteField("savepath", savePath)
	}
	_ = w.Close()

	baseURL := strings.TrimRight(a.cfg.Host, "/")
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
		baseURL+"/api/v2/torrents/add", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Referer", baseURL)
	req.Header.Set("Origin", baseURL)

	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("qbittorrent add torrent: %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if strings.EqualFold(strings.TrimSpace(string(raw)), "Fails.") {
		return "", fmt.Errorf("qbittorrent add torrent: rejected by downloader")
	}
	return "", nil
}

// AddMagnet 通过磁力链接添加种子。
func (a *QBitAdapter) AddMagnet(ctx context.Context, magnet, savePath string) (string, error) {
	return a.AddTorrent(ctx, magnet, savePath)
}

// Pause 暂停种子。
func (a *QBitAdapter) Pause(ctx context.Context, hash string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureAuthLocked(ctx); err != nil {
		return err
	}
	return a.postTorrentActionLocked(ctx, hash, "pause", "stop")
}

// Resume 恢复种子。
func (a *QBitAdapter) Resume(ctx context.Context, hash string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureAuthLocked(ctx); err != nil {
		return err
	}
	return a.postTorrentActionLocked(ctx, hash, "resume", "start")
}

func (a *QBitAdapter) postTorrentActionLocked(ctx context.Context, hash string, primary, fallback string) error {
	baseURL := strings.TrimRight(a.cfg.Host, "/")
	form := url.Values{}
	form.Set("hashes", hash)
	var lastErr error
	for _, action := range []string{primary, fallback} {
		if action == "" {
			continue
		}
		req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
			baseURL+"/api/v2/torrents/"+action, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", baseURL)
		req.Header.Set("Origin", baseURL)
		resp, err := a.client.Do(req)
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

// Remove 删除种子。
func (a *QBitAdapter) Remove(ctx context.Context, hash string, deleteFiles bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureAuthLocked(ctx); err != nil {
		return err
	}
	baseURL := strings.TrimRight(a.cfg.Host, "/")
	form := url.Values{}
	form.Set("hashes", hash)
	if deleteFiles {
		form.Set("deleteFiles", "true")
	} else {
		form.Set("deleteFiles", "false")
	}
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
		baseURL+"/api/v2/torrents/delete", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", baseURL)
	req.Header.Set("Origin", baseURL)
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("qbittorrent delete: %d", resp.StatusCode)
	}
	return nil
}

// loginLocked 执行登录（调用者必须持有锁）。
func (a *QBitAdapter) loginLocked(ctx context.Context) error {
	if a.cfg.Host == "" {
		return fmt.Errorf("qbittorrent host not configured")
	}
	if err := qbitLogin(ctx, a.client, a.cfg.Host, a.cfg.Username, a.cfg.Password); err != nil {
		return err
	}
	a.LoggedIn = true
	return nil
}

// ensureAuthLocked 确保已认证（调用者必须持有锁）。
func (a *QBitAdapter) ensureAuthLocked(ctx context.Context) error {
	if a.LoggedIn {
		return nil
	}
	return a.loginLocked(ctx)
}
