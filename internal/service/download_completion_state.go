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
	case "completed", "complete", "seeding", "uploading", "stalledup", "pausedup", "queuedup", "forcedup":
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
		owner := strings.ToLower(firstNonEmpty(torrent.ClientID, torrent.Source))
		if owner != "" {
			return owner + "|" + hash
		}
		return hash
	}
	parts := []string{torrent.Name, torrent.ContentPath, torrent.SavePath}
	if owner := firstNonEmpty(torrent.ClientID, torrent.Source); owner != "" {
		parts = append([]string{owner}, parts...)
	}
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
	if d == nil || d.repo == nil || d.repo.DB == nil {
		return
	}
	matched, ok := findMatchingTaskForTorrent(torrent, taskByKey)
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
	if strings.TrimSpace(matched.DownloadClientID) == "" && strings.TrimSpace(torrent.ClientID) != "" {
		updates["download_client_id"] = strings.TrimSpace(torrent.ClientID)
	}
	if strings.TrimSpace(matched.ExternalID) == "" && strings.TrimSpace(torrent.Hash) != "" {
		updates["external_id"] = strings.TrimSpace(torrent.Hash)
	}
	if strings.TrimSpace(torrent.Source) != "" && matched.Source != strings.TrimSpace(torrent.Source) {
		updates["source"] = strings.TrimSpace(torrent.Source)
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
	out := make(map[string]model.DownloadTask, len(rows)*4)
	for _, row := range rows {
		key := normalizeTorrentName(row.Title)
		if key != "" {
			setDownloadTaskIndex(out, key, row)
			setDownloadTaskIndex(out, downloadTaskClientTitleKey(row.DownloadClientID, key), row)
		}
		if externalID := strings.TrimSpace(row.ExternalID); externalID != "" {
			setDownloadTaskIndex(out, downloadTaskExternalKey(row.DownloadClientID, externalID), row)
			setDownloadTaskIndex(out, downloadTaskAnyExternalKey(externalID), row)
		}
	}
	return out
}

func setDownloadTaskIndex(index map[string]model.DownloadTask, key string, row model.DownloadTask) {
	if key == "" {
		return
	}
	if _, exists := index[key]; !exists {
		index[key] = row
	}
}

func downloadTaskExternalKey(clientID, externalID string) string {
	clientID = strings.ToLower(strings.TrimSpace(clientID))
	externalID = strings.ToLower(strings.TrimSpace(externalID))
	if clientID == "" || externalID == "" {
		return ""
	}
	return "\x00external:" + clientID + ":" + externalID
}

func downloadTaskAnyExternalKey(externalID string) string {
	externalID = strings.ToLower(strings.TrimSpace(externalID))
	if externalID == "" {
		return ""
	}
	return "\x00external-any:" + externalID
}

func downloadTaskClientTitleKey(clientID, titleKey string) string {
	clientID = strings.ToLower(strings.TrimSpace(clientID))
	titleKey = strings.TrimSpace(titleKey)
	if clientID == "" || titleKey == "" {
		return ""
	}
	return "\x00client-title:" + clientID + ":" + titleKey
}

func findMatchingTaskForTorrent(torrent QBitTorrent, taskByKey map[string]model.DownloadTask) (model.DownloadTask, bool) {
	if row, ok := taskByKey[downloadTaskExternalKey(torrent.ClientID, torrent.Hash)]; ok {
		return row, true
	}
	if row, ok := taskByKey[downloadTaskAnyExternalKey(torrent.Hash)]; ok {
		if strings.TrimSpace(row.DownloadClientID) == "" || strings.TrimSpace(torrent.ClientID) == "" || row.DownloadClientID == torrent.ClientID {
			return row, true
		}
	}
	titleKey := normalizeTorrentName(torrent.Name)
	if row, ok := taskByKey[downloadTaskClientTitleKey(torrent.ClientID, titleKey)]; ok {
		return row, true
	}
	row, ok := findMatchingTaskByTorrentIdentity(torrent.Name, taskByKey)
	if !ok {
		return model.DownloadTask{}, false
	}
	if strings.TrimSpace(torrent.ClientID) != "" && strings.TrimSpace(row.DownloadClientID) != "" && row.DownloadClientID != torrent.ClientID {
		return model.DownloadTask{}, false
	}
	return row, true
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
		if strings.HasPrefix(currentKey, "\x00") {
			continue
		}
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
		if strings.HasPrefix(currentKey, "\x00") {
			continue
		}
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
