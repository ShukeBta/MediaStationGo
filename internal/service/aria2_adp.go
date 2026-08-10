// Package service — Aria2 下载适配器。
//
// Aria2Adapter 实现了 DownloadAdapter 接口，通过 Aria2 JSON-RPC API
// 管理下载任务。
package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var errAria2ListUnavailable = errors.New("aria2 task list unavailable")

// Aria2Adapter 是 Aria2 的 DownloadAdapter 实现。
type Aria2Adapter struct {
	mu     sync.Mutex
	cfg    DownloadClientConfig
	client *http.Client
	idSeq  int
}

// NewAria2Adapter 创建新的 Aria2 适配器。
func NewAria2Adapter() *Aria2Adapter {
	return &Aria2Adapter{
		client: NewInternalHTTPClient(20 * time.Second),
	}
}

// AddTorrent 通过 URL 添加种子或磁力链接。
func (a *Aria2Adapter) AddTorrent(ctx context.Context, torrentURL, savePath string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Aria2 addUri 的参数: [secret, [uris], options]
	uris := []string{torrentURL}
	options := map[string]string{}
	if savePath != "" {
		options["dir"] = savePath
	}

	result, err := a.rpcLocked(ctx, "aria2.addUri", []interface{}{uris, options})
	if err != nil {
		return "", err
	}
	var gid string
	if err := json.Unmarshal(result, &gid); err != nil {
		return "", err
	}
	return gid, nil
}

// AddMagnet 通过磁力链接添加下载。
func (a *Aria2Adapter) AddMagnet(ctx context.Context, magnet, savePath string) (string, error) {
	return a.AddTorrent(ctx, magnet, savePath)
}

// AddTorrentFile submits application-fetched .torrent bytes using
// aria2.addTorrent. The empty URI list matches aria2's RPC signature.
func (a *Aria2Adapter) AddTorrentFile(ctx context.Context, data []byte, _ string, savePath string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	options := map[string]string{}
	if savePath != "" {
		options["dir"] = savePath
	}
	result, err := a.rpcLocked(ctx, "aria2.addTorrent", []interface{}{
		base64.StdEncoding.EncodeToString(data),
		[]string{},
		options,
	})
	if err != nil {
		return "", err
	}
	var gid string
	if err := json.Unmarshal(result, &gid); err != nil {
		return "", err
	}
	return gid, nil
}

// Pause 暂停下载任务（通过 GID）。
func (a *Aria2Adapter) Pause(ctx context.Context, hash string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.rpcLocked(ctx, "aria2.pause", []interface{}{hash})
	return err
}

// Resume 恢复下载任务（通过 GID）。
func (a *Aria2Adapter) Resume(ctx context.Context, hash string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, err := a.rpcLocked(ctx, "aria2.unpause", []interface{}{hash})
	return err
}

// Remove 移除下载任务。
func (a *Aria2Adapter) Remove(ctx context.Context, hash string, deleteFiles bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, removeErr := a.rpcLocked(ctx, "aria2.remove", []interface{}{hash})
	_, resultErr := a.rpcLocked(ctx, "aria2.removeDownloadResult", []interface{}{hash})
	if removeErr != nil && resultErr != nil {
		return errors.Join(removeErr, resultErr)
	}
	_ = deleteFiles // aria2 RPC removes the task/result but has no delete-local-data flag.
	return nil
}

// List 列出所有活动/等待/已停止的任务。
func (a *Aria2Adapter) List(ctx context.Context, filter string) ([]TorrentInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	var allResults []TorrentInfo
	var listErrs []error
	var successfulCalls int

	// 获取活动任务
	active, err := a.rpcLocked(ctx, "aria2.tellActive", []interface{}{
		[]string{"gid", "bittorrent", "files", "totalLength", "completedLength", "downloadSpeed", "uploadSpeed", "status", "dir", "numSeeders", "connections", "errorCode"},
	})
	if err == nil && active != nil {
		successfulCalls++
		items := a.parseAria2Items(active)
		allResults = append(allResults, items...)
	} else if err != nil {
		listErrs = append(listErrs, err)
	}

	// 获取等待中的任务
	waiting, err := a.rpcLocked(ctx, "aria2.tellWaiting", []interface{}{
		0, 100,
		[]string{"gid", "bittorrent", "files", "totalLength", "completedLength", "downloadSpeed", "uploadSpeed", "status", "dir", "numSeeders", "connections", "errorCode"},
	})
	if err == nil && waiting != nil {
		successfulCalls++
		items := a.parseAria2Items(waiting)
		allResults = append(allResults, items...)
	} else if err != nil {
		listErrs = append(listErrs, err)
	}

	// 获取已停止的任务
	stopped, err := a.rpcLocked(ctx, "aria2.tellStopped", []interface{}{
		0, 100,
		[]string{"gid", "bittorrent", "files", "totalLength", "completedLength", "downloadSpeed", "uploadSpeed", "status", "dir", "numSeeders", "connections", "errorCode"},
	})
	if err == nil && stopped != nil {
		successfulCalls++
		items := a.parseAria2Items(stopped)
		allResults = append(allResults, items...)
	} else if err != nil {
		listErrs = append(listErrs, err)
	}
	if successfulCalls == 0 {
		return nil, fmt.Errorf("%w: %w", errAria2ListUnavailable, errors.Join(listErrs...))
	}

	if filter != "" {
		filtered := make([]TorrentInfo, 0, len(allResults))
		for _, item := range allResults {
			if strings.EqualFold(item.State, filter) {
				filtered = append(filtered, item)
			}
		}
		return filtered, errors.Join(listErrs...)
	}

	return allResults, errors.Join(listErrs...)
}

// GetInfo 获取单个任务信息。
func (a *Aria2Adapter) GetInfo(ctx context.Context, hash string) (*TorrentInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	result, err := a.rpcLocked(ctx, "aria2.tellStatus", []interface{}{
		hash,
		[]string{"gid", "bittorrent", "files", "totalLength", "completedLength", "downloadSpeed", "uploadSpeed", "status", "dir", "numSeeders", "connections"},
	})
	if err != nil {
		return nil, err
	}

	var item map[string]interface{}
	if err := json.Unmarshal(result, &item); err != nil {
		return nil, err
	}

	info := a.parseSingleItem(item)
	if info == nil {
		return nil, fmt.Errorf("task %s not found", hash)
	}
	return info, nil
}
