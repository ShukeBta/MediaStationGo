package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const completedTorrentOrganizeQueueSize = 64

var completedTorrentOrganizeCooldown = 3 * time.Second

// poll fans out qBittorrent /torrents/info every 5 s as WS events. The
// payload is opaque to the client; the React store merges by hash.
func (d *DownloadService) poll(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	// prevStates tracks previous completion states to detect "just finished"
	if d.prevStates == nil {
		d.prevStates = make(map[string]bool)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case <-t.C:
		}
		live, err := d.qb.List(ctx, "")
		if err != nil {
			continue
		}
		rows, _ := d.repo.Download.List(ctx)
		taskByKey := tasksByTorrentIdentity(rows)
		d.processDownloadSnapshot(ctx, live, taskByKey)
		d.hub.Publish("download", map[string]any{"torrents": live})
	}
}

func (d *DownloadService) processDownloadSnapshot(ctx context.Context, live []QBitTorrent, taskByKey map[string]model.DownloadTask) {
	d.recordLiveTorrentSnapshot(live)
	firstSnapshot := d.beginDownloadSnapshot()
	for _, torrent := range live {
		d.processTorrentSnapshot(ctx, torrent, taskByKey, firstSnapshot)
	}
}

func (d *DownloadService) beginDownloadSnapshot() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.prevStates == nil {
		d.prevStates = make(map[string]bool)
	}
	firstSnapshot := !d.pollInitialized
	if firstSnapshot {
		d.pollInitialized = true
	}
	return firstSnapshot
}

func (d *DownloadService) processTorrentSnapshot(ctx context.Context, torrent QBitTorrent, taskByKey map[string]model.DownloadTask, firstSnapshot bool) {
	stateKey := completedTorrentQueueKey(torrent)
	taskNeedsOrganize := d.downloadSnapshotTaskNeedsOrganize(ctx, torrent, taskByKey)
	d.syncDownloadTaskProgress(ctx, torrent, taskByKey)
	if stateKey == "" {
		return
	}
	if d.completedTorrentShouldQueue(stateKey, qbitTorrentCompleted(torrent), firstSnapshot, taskNeedsOrganize) &&
		d.enqueueCompletedTorrent(torrent) {
		d.markCompletedTorrentState(stateKey)
	}
}

func (d *DownloadService) downloadSnapshotTaskNeedsOrganize(ctx context.Context, torrent QBitTorrent, taskByKey map[string]model.DownloadTask) bool {
	matchedTask, hasTask := findMatchingTaskByTorrentIdentity(torrent.Name, taskByKey)
	if !hasTask || d.completedTorrentCatchupRecorded(ctx, torrent) || !d.downloadAutoOrganizeEnabled(ctx) {
		return false
	}
	return downloadTaskNeedsCompletion(matchedTask) || recentlyCompletedTorrent(torrent, time.Now())
}

func (d *DownloadService) completedTorrentShouldQueue(stateKey string, complete, firstSnapshot, taskNeedsOrganize bool) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.prevStates == nil {
		d.prevStates = make(map[string]bool)
	}
	wasComplete, wasSeen := d.prevStates[stateKey]
	switch {
	case complete && (firstSnapshot || !wasSeen):
		// 首次快照里已完成的种子：此前一律标记「已见过」并跳过整理，
		// 导致「下载完成时应用恰好不在线/正在重启」的种子永远不会被
		// 自动整理入库。现在对最近完成的种子补一次整理
		// （onTorrentComplete 内部仍受 organize.auto 开关约束，且
		// 整理对已存在的目标文件幂等跳过）。
		d.prevStates[stateKey] = true
		return taskNeedsOrganize
	case complete && !wasComplete:
		return true
	case complete && taskNeedsOrganize:
		return true
	case complete:
		d.prevStates[stateKey] = true
	default:
		d.prevStates[stateKey] = false
	}
	return false
}

func (d *DownloadService) markCompletedTorrentState(stateKey string) {
	d.mu.Lock()
	d.prevStates[stateKey] = true
	d.mu.Unlock()
}

func (d *DownloadService) startAutoOrganizeWorker(ctx context.Context) {
	d.mu.Lock()
	if d.organizeQueue == nil {
		d.organizeQueue = make(chan QBitTorrent, completedTorrentOrganizeQueueSize)
	}
	if d.organizeQueued == nil {
		d.organizeQueued = make(map[string]struct{})
	}
	d.mu.Unlock()
	d.organizeOnce.Do(func() {
		go d.autoOrganizeWorker(ctx)
	})
}

func (d *DownloadService) enqueueCompletedTorrent(torrent QBitTorrent) bool {
	key := completedTorrentQueueKey(torrent)
	if key == "" {
		return false
	}
	d.mu.Lock()
	if d.organizeQueue == nil {
		d.organizeQueue = make(chan QBitTorrent, completedTorrentOrganizeQueueSize)
	}
	if d.organizeQueued == nil {
		d.organizeQueued = make(map[string]struct{})
	}
	if _, ok := d.organizeQueued[key]; ok {
		d.mu.Unlock()
		return true
	}
	select {
	case d.organizeQueue <- torrent:
		d.organizeQueued[key] = struct{}{}
		d.mu.Unlock()
		return true
	default:
		d.mu.Unlock()
		if d.log != nil {
			d.log.Warn("auto organize queue full; will retry completed torrent later",
				zap.String("hash", torrent.Hash),
				zap.String("name", torrent.Name))
		}
		return false
	}
}

func (d *DownloadService) autoOrganizeWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopCh:
			return
		case torrent := <-d.organizeQueue:
			d.onTorrentComplete(ctx, torrent)
			d.markCompletedTorrentOrganizeDone(torrent)
			if completedTorrentOrganizeCooldown <= 0 {
				continue
			}
			timer := time.NewTimer(completedTorrentOrganizeCooldown)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-d.stopCh:
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}
}

func (d *DownloadService) markCompletedTorrentOrganizeDone(torrent QBitTorrent) {
	key := completedTorrentQueueKey(torrent)
	if key == "" {
		return
	}
	d.mu.Lock()
	delete(d.organizeQueued, key)
	d.mu.Unlock()
}
