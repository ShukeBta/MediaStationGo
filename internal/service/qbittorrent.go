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
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"path"
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
	Name     string  `json:"name"`
	State    string  `json:"state"`
	Progress float32 `json:"progress"`
	DLSpeed  int64   `json:"dlspeed"`
	UpSpeed  int64   `json:"upspeed"`
	NumSeeds int     `json:"num_seeds"`
	NumLeech int     `json:"num_leechs"`
	Size     int64   `json:"size"`
	SavePath string  `json:"save_path"`
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

// NewQBitClient builds a fresh client, applying default URL if blank.
func NewQBitClient(log *zap.Logger, cfg QBitConfig) *QBitClient {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:8080"
	}
	jar, _ := cookiejar.New(nil)
	return &QBitClient{
		log:    log,
		cfg:    cfg,
		client: &http.Client{Jar: jar, Timeout: 20 * time.Second},
	}
}

// Configure rotates the client to a new endpoint and re-auths next call.
func (q *QBitClient) Configure(cfg QBitConfig) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.cfg = cfg
	jar, _ := cookiejar.New(nil)
	q.client.Jar = jar
}

// Login performs POST /api/v2/auth/login.
func (q *QBitClient) Login(ctx context.Context) error {
	if q.cfg.BaseURL == "" {
		return errors.New("qbittorrent base url not configured")
	}
	return qbitLogin(ctx, q.client, q.cfg.BaseURL, q.cfg.Username, q.cfg.Password)
}

// AddTorrent submits a magnet URL or HTTP(S) URL to qBittorrent.
//
// qBittorrent 的 /api/v2/torrents/add 在很多失败场景下仍然返回 HTTP 200
// 但 body 里写 "Fails."。我们把这些情况也识别为错误并返回，避免
// "API 返回 200 → 我们告诉前端成功 → qb 中却没下载" 这种迷惑性失败。
func (q *QBitClient) AddTorrent(ctx context.Context, magnetOrURL, savePath string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}

	torrentData, torrentName, fetchErr := q.fetchTorrentFile(ctx, magnetOrURL)
	useFileUpload := fetchErr == nil && len(torrentData) > 0
	return q.addTorrentLocked(ctx, magnetOrURL, torrentData, torrentName, useFileUpload, savePath)
}

func (q *QBitClient) AddTorrentFile(ctx context.Context, data []byte, name, savePath string) error {
	if len(data) == 0 {
		return errors.New("empty torrent data")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}
	return q.addTorrentLocked(ctx, "", data, name, true, savePath)
}

func (q *QBitClient) addTorrentLocked(ctx context.Context, magnetOrURL string, torrentData []byte, torrentName string, useFileUpload bool, savePath string) error {
	before, beforeErr := q.listLocked(ctx, "")
	beforeHashes := make(map[string]struct{}, len(before))
	if beforeErr == nil {
		for _, torrent := range before {
			if torrent.Hash != "" {
				beforeHashes[strings.ToLower(torrent.Hash)] = struct{}{}
			}
		}
	}
	if useFileUpload && beforeErr == nil {
		if hash := torrentInfoHash(torrentData); hash != "" {
			if _, ok := beforeHashes[hash]; ok {
				q.log.Info("qbittorrent: torrent already exists", zap.String("hash", hash), zap.String("name", torrentName))
				return nil
			}
		}
	}

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	if useFileUpload {
		if strings.TrimSpace(torrentName) == "" {
			torrentName = "download.torrent"
		}
		part, err := w.CreateFormFile("torrents", torrentName)
		if err != nil {
			return err
		}
		if _, err := part.Write(torrentData); err != nil {
			return err
		}
	} else {
		_ = w.WriteField("urls", magnetOrURL)
	}
	if savePath != "" {
		_ = w.WriteField("savepath", savePath)
	}
	_ = w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/add", body,
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Referer", q.cfg.BaseURL)
	req.Header.Set("Origin", q.cfg.BaseURL)

	resp, err := q.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	bodyText := strings.TrimSpace(string(raw))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("qbittorrent add: HTTP %d: %s", resp.StatusCode, bodyText)
	}
	// qb 的成功响应是 "Ok." 或空体；任何 "Fails." 视为失败。
	if strings.EqualFold(bodyText, "Fails.") {
		return fmt.Errorf("qbittorrent add: 拒绝任务 (检查 URL 是否需要认证或 savePath 是否可写)")
	}
	if beforeErr == nil {
		accepted := false
		var lastListErr error
		for attempt := 0; attempt < qbitAddVerifyAttempts; attempt++ {
			if attempt > 0 {
				time.Sleep(qbitAddVerifyInterval)
			}
			after, err := q.listLocked(ctx, "")
			if err != nil {
				lastListErr = err
				continue
			}
			for _, torrent := range after {
				if torrent.Hash == "" {
					continue
				}
				if _, ok := beforeHashes[torrent.Hash]; !ok {
					accepted = true
					break
				}
			}
			if accepted {
				break
			}
		}
		if !accepted {
			if lastListErr != nil {
				return fmt.Errorf("qbittorrent add: 无法确认任务已加入下载器: %w", lastListErr)
			}
			return fmt.Errorf("qbittorrent add: 下载器未出现新任务，可能种子已存在或 URL 未被下载器接受")
		}
	}
	q.log.Info("qbittorrent: torrent added",
		zap.String("url", redactTorrentURL(magnetOrURL)),
		zap.String("save_path", savePath),
		zap.Bool("file_upload", useFileUpload),
		zap.String("body", bodyText))
	return nil
}

func redactTorrentURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(raw), "magnet:") {
		return "magnet:?xt=***"
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "[redacted-download-url]"
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func (q *QBitClient) fetchTorrentFile(ctx context.Context, raw string) ([]byte, string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return nil, "", errors.New("not a remote URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, "", errors.New("not an HTTP torrent URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "MediaStationGo/0.1")
	req.Header.Set("Accept", "application/x-bittorrent,application/octet-stream,*/*")

	client := NewExternalHTTPClient(30 * time.Second)
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("torrent fetch: HTTP %d", resp.StatusCode)
	}

	const maxTorrentSize = 32 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTorrentSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", errors.New("torrent fetch: empty body")
	}
	if len(data) > maxTorrentSize {
		return nil, "", errors.New("torrent fetch: body too large")
	}
	if strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/html") {
		return nil, "", errors.New("torrent fetch: upstream returned HTML")
	}

	name := strings.TrimSpace(path.Base(u.Path))
	if name == "" || name == "." || name == "/" {
		name = "download.torrent"
	}
	if !strings.HasSuffix(strings.ToLower(name), ".torrent") {
		name += ".torrent"
	}
	return data, name, nil
}

func torrentInfoHash(data []byte) string {
	start, end, ok := torrentInfoBounds(data)
	if !ok {
		return ""
	}
	sum := sha1.Sum(data[start:end])
	return hex.EncodeToString(sum[:])
}

func torrentInfoBounds(data []byte) (int, int, bool) {
	if len(data) == 0 || data[0] != 'd' {
		return 0, 0, false
	}
	pos := 1
	for pos < len(data) && data[pos] != 'e' {
		keyStart, keyEnd, next, ok := parseBencodeString(data, pos)
		if !ok {
			return 0, 0, false
		}
		valueStart := next
		valueEnd, ok := bencodeValueEnd(data, valueStart)
		if !ok {
			return 0, 0, false
		}
		if string(data[keyStart:keyEnd]) == "info" {
			return valueStart, valueEnd, true
		}
		pos = valueEnd
	}
	return 0, 0, false
}

func parseBencodeString(data []byte, pos int) (int, int, int, bool) {
	if pos >= len(data) || data[pos] < '0' || data[pos] > '9' {
		return 0, 0, 0, false
	}
	length := 0
	for pos < len(data) && data[pos] >= '0' && data[pos] <= '9' {
		length = length*10 + int(data[pos]-'0')
		pos++
	}
	if pos >= len(data) || data[pos] != ':' {
		return 0, 0, 0, false
	}
	start := pos + 1
	end := start + length
	if end > len(data) {
		return 0, 0, 0, false
	}
	return start, end, end, true
}

func bencodeValueEnd(data []byte, pos int) (int, bool) {
	if pos >= len(data) {
		return 0, false
	}
	switch data[pos] {
	case 'i':
		end := pos + 1
		for end < len(data) && data[end] != 'e' {
			end++
		}
		if end >= len(data) {
			return 0, false
		}
		return end + 1, true
	case 'l', 'd':
		end := pos + 1
		for end < len(data) && data[end] != 'e' {
			next, ok := bencodeValueEnd(data, end)
			if !ok {
				return 0, false
			}
			end = next
		}
		if end >= len(data) {
			return 0, false
		}
		return end + 1, true
	default:
		_, _, next, ok := parseBencodeString(data, pos)
		return next, ok
	}
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
	if filter != "" {
		u += "?filter=" + url.QueryEscape(filter)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/delete",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.cfg.BaseURL)
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/setLocation",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", q.cfg.BaseURL)
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
