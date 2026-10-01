package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

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
		// qB's default automatic management may replace this path with the
		// category's configured path. Preserve the destination chosen here.
		_ = w.WriteField("autoTMM", "false")
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
