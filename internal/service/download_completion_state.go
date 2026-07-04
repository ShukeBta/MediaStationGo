package service

import (
	"context"
	"crypto/sha1"
	"fmt"
	"math"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// completedTorrentCatchupWindow 限定重启补整理只覆盖最近完成的种子，
// 防止每次启动都把全部历史种子重新过一遍整理流程。
const completedTorrentCatchupWindow = 24 * time.Hour

const completedTorrentCatchupSettingPrefix = "download.auto_organized."
const completedTorrentNotifySettingPrefix = "download.completed_notified."

func (d *DownloadService) downloadAutoOrganizeEnabled(ctx context.Context) bool {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return false
	}
	if v, err := d.repo.Setting.Get(ctx, "organizer.auto_after_download"); err == nil && parseBoolSetting(v, false) {
		return true
	}
	if v, err := d.repo.Setting.Get(ctx, "organize.auto"); err == nil && parseBoolSetting(v, false) {
		return true
	}
	return false
}

// recentlyCompletedTorrent 报告该种子是否在补整理时间窗内完成。
// qBittorrent 未提供 completion_on 时保守地返回 false。
func recentlyCompletedTorrent(torrent QBitTorrent, now time.Time) bool {
	if torrent.CompletionOn <= 0 {
		return false
	}
	completed := time.Unix(torrent.CompletionOn, 0)
	return now.Sub(completed) <= completedTorrentCatchupWindow
}

func qbitTorrentCompleted(torrent QBitTorrent) bool {
	if torrent.Progress < 1 {
		return false
	}
	state := strings.ToLower(strings.TrimSpace(torrent.State))
	switch state {
	case "completed", "uploading", "stalledup", "pausedup", "queuedup", "forcedup":
		return true
	default:
		return false
	}
}

func (d *DownloadService) completedTorrentCatchupRecorded(ctx context.Context, torrent QBitTorrent) bool {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return false
	}
	key := completedTorrentCatchupSettingKey(torrent)
	if key == "" {
		return false
	}
	value, err := d.repo.Setting.Get(ctx, key)
	if err != nil {
		return false
	}
	return parseBoolSetting(value, false)
}

func (d *DownloadService) markCompletedTorrentCatchupRecorded(ctx context.Context, torrent QBitTorrent) {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return
	}
	key := completedTorrentCatchupSettingKey(torrent)
	if key == "" {
		return
	}
	if err := d.repo.Setting.Set(ctx, key, "true"); err != nil && d.log != nil {
		d.log.Debug("mark completed torrent catchup failed",
			zap.String("hash", torrent.Hash),
			zap.String("name", torrent.Name),
			zap.Error(err))
	}
}

func completedTorrentCatchupSettingKey(torrent QBitTorrent) string {
	key := completedTorrentQueueKey(torrent)
	if key == "" {
		return ""
	}
	sum := sha1.Sum([]byte(key))
	return completedTorrentCatchupSettingPrefix + fmt.Sprintf("%x", sum[:])
}

func (d *DownloadService) completedTorrentNotified(ctx context.Context, torrent QBitTorrent) bool {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return false
	}
	key := completedTorrentNotifySettingKey(torrent)
	if key == "" {
		return false
	}
	value, err := d.repo.Setting.Get(ctx, key)
	if err != nil {
		return false
	}
	return parseBoolSetting(value, false)
}

func (d *DownloadService) markCompletedTorrentNotified(ctx context.Context, torrent QBitTorrent) {
	if d == nil || d.repo == nil || d.repo.Setting == nil {
		return
	}
	key := completedTorrentNotifySettingKey(torrent)
	if key == "" {
		return
	}
	if err := d.repo.Setting.Set(ctx, key, "true"); err != nil && d.log != nil {
		d.log.Debug("mark completed torrent notification failed",
			zap.String("hash", torrent.Hash),
			zap.String("name", torrent.Name),
			zap.Error(err))
	}
}

func completedTorrentNotifySettingKey(torrent QBitTorrent) string {
	key := completedTorrentQueueKey(torrent)
	if key == "" {
		return ""
	}
	sum := sha1.Sum([]byte(key))
	return completedTorrentNotifySettingPrefix + fmt.Sprintf("%x", sum[:])
}

func completedTorrentQueueKey(torrent QBitTorrent) string {
	hash := strings.ToLower(strings.TrimSpace(torrent.Hash))
	if hash != "" {
		return hash
	}
	parts := []string{torrent.Name, torrent.ContentPath, torrent.SavePath}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	key := strings.Join(parts, "|")
	if strings.Trim(key, "|") == "" {
		return ""
	}
	return strings.ToLower(key)
}

func (d *DownloadService) syncDownloadTaskProgress(ctx context.Context, torrent QBitTorrent, taskByKey map[string]model.DownloadTask) {
	if d == nil || d.repo == nil || d.repo.DB == nil || strings.TrimSpace(torrent.Name) == "" {
		return
	}
	matched, ok := findMatchingTaskByTorrentIdentity(torrent.Name, taskByKey)
	if !ok {
		return
	}
	status := torrent.State
	if qbitTorrentCompleted(torrent) {
		status = "completed"
	}
	if strings.TrimSpace(status) == "" {
		status = matched.Status
	}
	updates := map[string]any{}
	if math.Abs(float64(matched.Progress-torrent.Progress)) > 0.0001 {
		updates["progress"] = torrent.Progress
	}
	if status != "" && status != matched.Status {
		updates["status"] = status
	}
	if len(updates) == 0 {
		return
	}
	_ = d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).Where("id = ?", matched.ID).Updates(updates).Error
}

func tasksByIdentity(rows []model.DownloadTask) map[string]model.DownloadTask {
	out := make(map[string]model.DownloadTask, len(rows))
	for _, row := range rows {
		key := downloadTaskIdentityKey(row.Title)
		if key != "" {
			out[key] = row
		}
	}
	return out
}

func tasksByTorrentIdentity(rows []model.DownloadTask) map[string]model.DownloadTask {
	out := make(map[string]model.DownloadTask, len(rows))
	for _, row := range rows {
		key := normalizeTorrentName(row.Title)
		if key != "" {
			out[key] = row
		}
	}
	return out
}

func findMatchingTaskByIdentity(title string, taskByKey map[string]model.DownloadTask) (model.DownloadTask, bool) {
	key := downloadTaskIdentityKey(title)
	if key == "" {
		return model.DownloadTask{}, false
	}
	if row, ok := taskByKey[key]; ok {
		return row, true
	}
	for currentKey, row := range taskByKey {
		if strings.Contains(key, currentKey) || strings.Contains(currentKey, key) {
			return row, true
		}
	}
	return model.DownloadTask{}, false
}

func findMatchingTaskByTorrentIdentity(title string, taskByKey map[string]model.DownloadTask) (model.DownloadTask, bool) {
	key := normalizeTorrentName(title)
	if key == "" {
		return model.DownloadTask{}, false
	}
	if row, ok := taskByKey[key]; ok {
		return row, true
	}
	for currentKey, row := range taskByKey {
		if strings.Contains(key, currentKey) || strings.Contains(currentKey, key) {
			return row, true
		}
	}
	return model.DownloadTask{}, false
}

func downloadTaskNeedsCompletion(task model.DownloadTask) bool {
	if task.Progress < 1 {
		return true
	}
	return strings.ToLower(strings.TrimSpace(task.Status)) != "completed"
}
