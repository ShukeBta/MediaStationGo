// Package service — Transmission 下载适配器。
//
// TransmissionAdapter 实现了 DownloadAdapter 接口，通过 Transmission RPC API
// 管理下载任务。
package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// TransmissionAdapter 是 Transmission 的 DownloadAdapter 实现。
type TransmissionAdapter struct {
	mu        sync.Mutex
	cfg       DownloadClientConfig
	client    *http.Client
	tag       int
	sessionID string
}

// NewTransmissionAdapter 创建新的 Transmission 适配器。
func NewTransmissionAdapter() *TransmissionAdapter {
	return &TransmissionAdapter{
		client: NewInternalHTTPClient(20 * time.Second),
	}
}

// AddTorrent 通过 URL 添加种子。
func (a *TransmissionAdapter) AddTorrent(ctx context.Context, torrentURL, savePath string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	args := map[string]interface{}{"filename": torrentURL}
	if savePath != "" {
		args["download-dir"] = savePath
	}
	resp, err := a.rpcLocked(ctx, "torrent-add", args)
	if err != nil {
		return "", err
	}
	if added, ok := resp.Arguments["torrent-added"].(map[string]interface{}); ok {
		if hashStr, ok := added["hashString"].(string); ok {
			return hashStr, nil
		}
	}
	if dup, ok := resp.Arguments["torrent-duplicate"].(map[string]interface{}); ok {
		if hashStr, ok := dup["hashString"].(string); ok {
			return hashStr, nil
		}
	}
	return "", nil
}

// AddMagnet 通过磁力链接添加种子。
func (a *TransmissionAdapter) AddMagnet(ctx context.Context, magnet, savePath string) (string, error) {
	return a.AddTorrent(ctx, magnet, savePath)
}

// Pause 暂停种子。
func (a *TransmissionAdapter) Pause(ctx context.Context, hash string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.rpcLocked(ctx, "torrent-stop", map[string]interface{}{
		"ids": []string{hash},
	})
	return err
}

// Resume 恢复种子。
func (a *TransmissionAdapter) Resume(ctx context.Context, hash string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.rpcLocked(ctx, "torrent-start", map[string]interface{}{
		"ids": []string{hash},
	})
	return err
}

// Remove 删除种子。
func (a *TransmissionAdapter) Remove(ctx context.Context, hash string, deleteFiles bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.rpcLocked(ctx, "torrent-remove", map[string]interface{}{
		"ids":               []string{hash},
		"delete-local-data": deleteFiles,
	})
	return err
}

// List 列出种子。
func (a *TransmissionAdapter) List(ctx context.Context, filter string) ([]TorrentInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	args := map[string]interface{}{
		"fields": []string{
			"hashString", "name", "totalSize", "percentDone",
			"rateDownload", "rateUpload", "status", "downloadDir",
			"peersSendingToUs", "peersGettingFromUs", "addedDate",
			"labels", "isStalled",
		},
	}
	resp, err := a.rpcLocked(ctx, "torrent-get", args)
	if err != nil {
		return nil, err
	}

	torrentsRaw, ok := resp.Arguments["torrents"].([]interface{})
	if !ok {
		return nil, nil
	}

	result := make([]TorrentInfo, 0, len(torrentsRaw))
	for _, tr := range torrentsRaw {
		t, ok := tr.(map[string]interface{})
		if !ok {
			continue
		}

		hash, _ := t["hashString"].(string)
		name, _ := t["name"].(string)
		size := toInt64(t["totalSize"])
		progress := toFloat64(t["percentDone"])
		dlSpeed := toInt64(t["rateDownload"])
		upSpeed := toInt64(t["rateUpload"])
		savePath, _ := t["downloadDir"].(string)
		numSeeds := int(toInt64(t["peersSendingToUs"]))
		numLeechs := int(toInt64(t["peersGettingFromUs"]))
		addedOn := int64(toFloat64(t["addedDate"]))

		// Transmission 状态码转字符串
		status := int(toFloat64(t["status"]))
		state := transmissionStateStr(status)

		// 过滤
		if filter != "" && !strings.EqualFold(state, filter) {
			continue
		}

		result = append(result, TorrentInfo{
			Hash:      hash,
			Name:      name,
			Size:      size,
			Progress:  progress * 100,
			DLSpeed:   dlSpeed,
			UPSpeed:   upSpeed,
			State:     state,
			SavePath:  savePath,
			NumSeeds:  numSeeds,
			NumLeechs: numLeechs,
			AddedOn:   time.Unix(addedOn, 0),
			Tags:      toJSONLabels(t["labels"]),
		})
	}
	return result, nil
}

// GetInfo 获取单个种子信息。
func (a *TransmissionAdapter) GetInfo(ctx context.Context, hash string) (*TorrentInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	args := map[string]interface{}{
		"ids": []string{hash},
		"fields": []string{
			"hashString", "name", "totalSize", "percentDone",
			"rateDownload", "rateUpload", "status", "downloadDir",
			"peersSendingToUs", "peersGettingFromUs", "addedDate", "labels",
		},
	}
	resp, err := a.rpcLocked(ctx, "torrent-get", args)
	if err != nil {
		return nil, err
	}
	torrentsRaw, ok := resp.Arguments["torrents"].([]interface{})
	if !ok || len(torrentsRaw) == 0 {
		return nil, fmt.Errorf("torrent %s not found", hash)
	}
	t, ok := torrentsRaw[0].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("torrent %s: invalid response", hash)
	}

	status := int(toFloat64(t["status"]))
	info := &TorrentInfo{
		Hash:      hash,
		Name:      strVal(t["name"]),
		Size:      toInt64(t["totalSize"]),
		Progress:  toFloat64(t["percentDone"]) * 100,
		DLSpeed:   toInt64(t["rateDownload"]),
		UPSpeed:   toInt64(t["rateUpload"]),
		State:     transmissionStateStr(status),
		SavePath:  strVal(t["downloadDir"]),
		NumSeeds:  int(toInt64(t["peersSendingToUs"])),
		NumLeechs: int(toInt64(t["peersGettingFromUs"])),
		AddedOn:   time.Unix(int64(toFloat64(t["addedDate"])), 0),
		Tags:      toJSONLabels(t["labels"]),
	}
	return info, nil
}
