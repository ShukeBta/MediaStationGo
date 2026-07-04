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
	"crypto/sha1" // #nosec G505 -- BitTorrent v1 info-hash is SHA-1 by protocol.
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
	"regexp"
	"strconv"
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
	Category string  `json:"category"`
	AddedOn  int64   `json:"added_on"`
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

type QBitTorrentFile struct {
	Index    int    `json:"index"`
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Priority int    `json:"priority"`
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
	qbitFileListAttempts  = 10
	qbitFileListInterval  = 500 * time.Millisecond
	qbitFileSizeSuffixRE  = regexp.MustCompile(`\s+\([0-9]+(?:\.[0-9]+)?\s*[KMGT]?B\)$`)
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

// AddTorrent submits a magnet URL or HTTP(S) URL to qBittorrent.
//
// qBittorrent 的 /api/v2/torrents/add 在很多失败场景下仍然返回 HTTP 200
// 但 body 里写 "Fails."。我们把这些情况也识别为错误并返回，避免
// "API 返回 200 → 我们告诉前端成功 → qb 中却没下载" 这种迷惑性失败。
func (q *QBitClient) AddTorrent(ctx context.Context, magnetOrURL, savePath string) error {
	return q.AddTorrentWithCategory(ctx, magnetOrURL, savePath, "")
}

func (q *QBitClient) AddTorrentWithCategory(ctx context.Context, magnetOrURL, savePath, category string) error {
	return q.AddTorrentWithCategoryAndName(ctx, magnetOrURL, savePath, category, "", nil)
}

func (q *QBitClient) AddTorrentWithCategoryAndName(ctx context.Context, magnetOrURL, savePath, category, displayName string, selectedFiles []string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}

	torrentData, torrentName, fetchErr := q.fetchTorrentFile(ctx, magnetOrURL)
	useFileUpload := fetchErr == nil && len(torrentData) > 0
	_, err := q.addTorrentLocked(ctx, magnetOrURL, torrentData, torrentName, useFileUpload, savePath, category, displayName, len(selectedFiles) > 0, selectedFiles)
	return err
}

func (q *QBitClient) AddTorrentFile(ctx context.Context, data []byte, name, savePath string) error {
	return q.AddTorrentFileWithCategory(ctx, data, name, savePath, "")
}

func (q *QBitClient) AddTorrentFileWithCategory(ctx context.Context, data []byte, name, savePath, category string) error {
	return q.AddTorrentFileWithCategoryAndName(ctx, data, name, savePath, category, "", nil)
}

func (q *QBitClient) AddTorrentFileWithCategoryAndName(ctx context.Context, data []byte, name, savePath, category, displayName string, selectedFiles []string) error {
	if len(data) == 0 {
		return errors.New("empty torrent data")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}
	_, err := q.addTorrentLocked(ctx, "", data, name, true, savePath, category, displayName, len(selectedFiles) > 0, selectedFiles)
	return err
}

func (q *QBitClient) PrepareTorrentWithCategoryAndName(ctx context.Context, magnetOrURL, savePath, category, displayName string) (string, []QBitTorrentFile, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return "", nil, err
	}

	torrentData, torrentName, fetchErr := q.fetchTorrentFile(ctx, magnetOrURL)
	useFileUpload := fetchErr == nil && len(torrentData) > 0
	hash, err := q.addTorrentLocked(ctx, magnetOrURL, torrentData, torrentName, useFileUpload, savePath, category, displayName, true, nil)
	if err != nil {
		return hash, nil, err
	}
	files, err := q.waitTorrentFilesLocked(ctx, hash)
	if err != nil {
		return hash, nil, err
	}
	return hash, files, nil
}

func (q *QBitClient) PrepareTorrentFileWithCategoryAndName(ctx context.Context, data []byte, name, savePath, category, displayName string) (string, []QBitTorrentFile, error) {
	if len(data) == 0 {
		return "", nil, errors.New("empty torrent data")
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return "", nil, err
	}
	hash, err := q.addTorrentLocked(ctx, "", data, name, true, savePath, category, displayName, true, nil)
	if err != nil {
		return hash, nil, err
	}
	files, err := q.waitTorrentFilesLocked(ctx, hash)
	if err != nil {
		return hash, nil, err
	}
	return hash, files, nil
}

func (q *QBitClient) addTorrentLocked(ctx context.Context, magnetOrURL string, torrentData []byte, torrentName string, useFileUpload bool, savePath, category, displayName string, paused bool, selectedFiles []string) (string, error) {
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
				return hash, ErrDownloadAlreadyExists
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
			return "", err
		}
		if _, err := part.Write(torrentData); err != nil {
			return "", err
		}
	} else {
		_ = w.WriteField("urls", magnetOrURL)
	}
	if savePath != "" {
		_ = w.WriteField("savepath", savePath)
	}
	if strings.TrimSpace(category) != "" {
		_ = w.WriteField("category", sanitizeQBitCategory(category))
	}
	if name := sanitizeQBitTorrentName(displayName); name != "" {
		_ = w.WriteField("rename", name)
	}
	if paused {
		_ = w.WriteField("paused", "true")
		_ = w.WriteField("stopped", "true")
	}
	_ = w.Close()

	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/add", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Referer", q.cfg.BaseURL)
	req.Header.Set("Origin", q.cfg.BaseURL)

	resp, err := q.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	bodyText := strings.TrimSpace(string(raw))
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("qbittorrent add: HTTP %d: %s", resp.StatusCode, bodyText)
	}
	// qb 的成功响应是 "Ok." 或空体；任何 "Fails." 视为失败。
	if strings.EqualFold(bodyText, "Fails.") {
		return "", fmt.Errorf("qbittorrent add: 拒绝任务 (检查 URL 是否需要认证或 savePath 是否可写)")
	}
	addedHash := ""
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
					addedHash = strings.ToLower(torrent.Hash)
					break
				}
			}
			if accepted {
				break
			}
		}
		if !accepted {
			if lastListErr != nil {
				return "", fmt.Errorf("qbittorrent add: 无法确认任务已加入下载器: %w", lastListErr)
			}
			return "", fmt.Errorf("qbittorrent add: 下载器未出现新任务，可能种子已存在或 URL 未被下载器接受")
		}
	}
	if addedHash != "" && len(selectedFiles) > 0 {
		if err := q.applySelectedFilesLocked(ctx, addedHash, selectedFiles); err != nil {
			return addedHash, err
		}
	}
	q.log.Info("qbittorrent: torrent added",
		zap.String("url", redactTorrentURL(magnetOrURL)),
		zap.String("save_path", savePath),
		zap.String("category", sanitizeQBitCategory(category)),
		zap.Bool("file_upload", useFileUpload),
		zap.String("body", bodyText))
	return addedHash, nil
}

func sanitizeQBitCategory(category string) string {
	return strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(category, "\r", " "), "\n", " "))
}

func (q *QBitClient) applySelectedFilesLocked(ctx context.Context, hash string, selectedFiles []string) error {
	files, err := q.torrentFilesLocked(ctx, hash)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	selected := selectedFileKeySet(selectedFiles)
	if len(selected) == 0 {
		return nil
	}
	skipIDs := make([]string, 0, len(files))
	for _, file := range files {
		if selectedFileMatches(file.Name, selected) {
			continue
		}
		skipIDs = append(skipIDs, strconv.Itoa(file.Index))
	}
	if len(skipIDs) > 0 {
		if err := q.setFilePriorityLocked(ctx, hash, skipIDs, 0); err != nil {
			return err
		}
	}
	return q.resumeLocked(ctx, hash)
}

func (q *QBitClient) ApplySelectedFileIndexes(ctx context.Context, hash string, selectedIndexes []int) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}
	files, err := q.torrentFilesLocked(ctx, hash)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return q.resumeLocked(ctx, hash)
	}
	selected := make(map[int]struct{}, len(selectedIndexes))
	for _, idx := range selectedIndexes {
		if idx >= 0 {
			selected[idx] = struct{}{}
		}
	}
	if len(selected) == 0 {
		return errors.New("no torrent files selected")
	}
	skipIDs := make([]string, 0, len(files))
	for _, file := range files {
		if _, ok := selected[file.Index]; ok {
			continue
		}
		skipIDs = append(skipIDs, strconv.Itoa(file.Index))
	}
	if len(skipIDs) > 0 {
		if err := q.setFilePriorityLocked(ctx, hash, skipIDs, 0); err != nil {
			return err
		}
	}
	return q.resumeLocked(ctx, hash)
}

func (q *QBitClient) Resume(ctx context.Context, hash string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return err
	}
	return q.resumeLocked(ctx, hash)
}

func (q *QBitClient) waitTorrentFilesLocked(ctx context.Context, hash string) ([]QBitTorrentFile, error) {
	if strings.TrimSpace(hash) == "" {
		return nil, nil
	}
	var lastErr error
	for attempt := 0; attempt < qbitFileListAttempts; attempt++ {
		if attempt > 0 {
			time.Sleep(qbitFileListInterval)
		}
		files, err := q.torrentFilesLocked(ctx, hash)
		if err != nil {
			lastErr = err
			continue
		}
		if len(files) > 0 {
			return files, nil
		}
	}
	return nil, lastErr
}

func (q *QBitClient) Files(ctx context.Context, hash string) ([]QBitTorrentFile, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureAuth(ctx); err != nil {
		return nil, err
	}
	return q.waitTorrentFilesLocked(ctx, hash)
}

func (q *QBitClient) torrentFilesLocked(ctx context.Context, hash string) ([]QBitTorrentFile, error) {
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodGet,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/files", nil)
	if err != nil {
		return nil, err
	}
	query := req.URL.Query()
	query.Set("hash", hash)
	req.URL.RawQuery = query.Encode()
	req.Header.Set("Referer", q.cfg.BaseURL)
	resp, err := q.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("qbittorrent files: %d", resp.StatusCode)
	}
	var files []QBitTorrentFile
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}
	return files, nil
}

func (q *QBitClient) setFilePriorityLocked(ctx context.Context, hash string, ids []string, priority int) error {
	form := url.Values{}
	form.Set("hash", hash)
	form.Set("id", strings.Join(ids, "|"))
	form.Set("priority", strconv.Itoa(priority))
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/filePrio",
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
		return fmt.Errorf("qbittorrent filePrio: %d", resp.StatusCode)
	}
	return nil
}

func (q *QBitClient) resumeLocked(ctx context.Context, hash string) error {
	if err := q.postTorrentActionLocked(ctx, hash, "resume"); err == nil {
		return nil
	}
	return q.postTorrentActionLocked(ctx, hash, "start")
}

func (q *QBitClient) postTorrentActionLocked(ctx context.Context, hash, action string) error {
	form := url.Values{}
	form.Set("hashes", hash)
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/"+action,
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
		return fmt.Errorf("qbittorrent %s: %d", action, resp.StatusCode)
	}
	return nil
}

func selectedFileKeySet(files []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, file := range files {
		for _, key := range selectedFileKeys(file) {
			out[key] = struct{}{}
		}
	}
	return out
}

func selectedFileMatches(name string, selected map[string]struct{}) bool {
	for _, key := range selectedFileKeys(name) {
		if _, ok := selected[key]; ok {
			return true
		}
	}
	return false
}

func selectedFileKeys(name string) []string {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	cleaned := strings.TrimSpace(qbitFileSizeSuffixRE.ReplaceAllString(name, ""))
	normalized := normalizeTorrentName(cleaned)
	base := normalizeTorrentName(path.Base(strings.ReplaceAll(cleaned, "\\", "/")))
	return compactUniqueStrings(normalized, base)
}

func sanitizeQBitTorrentName(name string) string {
	name = strings.TrimSpace(strings.Join(strings.Fields(name), " "))
	if name == "" {
		return ""
	}
	replacer := strings.NewReplacer("/", " ", "\\", " ", ":", " ", "*", " ", "?", " ", `"`, " ", "<", " ", ">", " ", "|", " ")
	return strings.TrimSpace(strings.Join(strings.Fields(replacer.Replace(name)), " "))
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
	sum := sha1.Sum(data[start:end]) // #nosec G401 -- BitTorrent v1 info-hash is SHA-1 by protocol, not a security hash.
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
