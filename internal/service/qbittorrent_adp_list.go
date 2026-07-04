package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// List 列出种子。
func (a *QBitAdapter) List(ctx context.Context, filter string) ([]TorrentInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.ensureAuthLocked(ctx); err != nil {
		return nil, err
	}
	baseURL := strings.TrimRight(a.cfg.Host, "/")
	u := baseURL + "/api/v2/torrents/info"
	req, err := newDownloadClientHTTPRequest(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if filter != "" {
		query := req.URL.Query()
		query.Set("filter", filter)
		req.URL.RawQuery = query.Encode()
	}
	req.Header.Set("Referer", baseURL)
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("qbittorrent list: %d", resp.StatusCode)
	}

	var qbList []qbitTorrentListItem
	if err := json.NewDecoder(resp.Body).Decode(&qbList); err != nil {
		return nil, err
	}
	return qbitTorrentListToInfo(qbList), nil
}

// GetInfo 获取单个种子信息。
func (a *QBitAdapter) GetInfo(ctx context.Context, hash string) (*TorrentInfo, error) {
	list, err := a.List(ctx, "")
	if err != nil {
		return nil, err
	}
	for _, t := range list {
		if t.Hash == hash {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("torrent %s not found", hash)
}

type qbitTorrentListItem struct {
	Hash      string  `json:"hash"`
	Name      string  `json:"name"`
	State     string  `json:"state"`
	Progress  float32 `json:"progress"`
	DLSpeed   int64   `json:"dlspeed"`
	UPSpeed   int64   `json:"upspeed"`
	NumSeeds  int     `json:"num_seeds"`
	NumLeechs int     `json:"num_leechs"`
	Size      int64   `json:"size"`
	SavePath  string  `json:"save_path"`
	AddedOn   int64   `json:"added_on"`
	Category  string  `json:"category"`
	Tags      string  `json:"tags"`
}

func qbitTorrentListToInfo(items []qbitTorrentListItem) []TorrentInfo {
	result := make([]TorrentInfo, 0, len(items))
	for _, item := range items {
		result = append(result, TorrentInfo{
			Hash:      item.Hash,
			Name:      item.Name,
			Size:      item.Size,
			Progress:  float64(item.Progress),
			DLSpeed:   item.DLSpeed,
			UPSpeed:   item.UPSpeed,
			State:     item.State,
			SavePath:  item.SavePath,
			NumSeeds:  item.NumSeeds,
			NumLeechs: item.NumLeechs,
			AddedOn:   time.Unix(item.AddedOn, 0),
			Category:  item.Category,
			Tags:      item.Tags,
		})
	}
	return result
}
