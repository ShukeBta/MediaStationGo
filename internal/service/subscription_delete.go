package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func (s *SubscriptionService) deleteSubscriptionDownloads(ctx context.Context, sub *model.Subscription) error {
	if s == nil || s.repo == nil || s.repo.Download == nil || sub == nil {
		return nil
	}
	rows, err := s.repo.Download.List(ctx)
	if err != nil {
		return err
	}
	candidates := make([]model.DownloadTask, 0)
	for _, row := range rows {
		if subscriptionDeleteMatchesTask(ctx, s, sub, row) {
			candidates = append(candidates, row)
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	var live []QBitTorrent
	if s.downloads != nil {
		live, _ = s.downloads.listLiveTorrents(ctx, "")
	}
	deletedHashes := map[string]struct{}{}
	for _, task := range candidates {
		hash := firstNonEmpty(task.ExternalID, downloadTaskInfoHash(task))
		clientID := strings.TrimSpace(task.DownloadClientID)
		if matched, ok := matchingLiveTorrent(task, live); ok {
			hash = firstNonEmpty(hash, matched.Hash)
			clientID = firstNonEmpty(clientID, matched.ClientID)
		}
		if hash != "" && s.downloads != nil {
			key := strings.ToLower(clientID + ":" + hash)
			if _, ok := deletedHashes[key]; !ok {
				if err := s.downloads.Delete(ctx, hash, false, clientID); err != nil {
					return fmt.Errorf("删除订阅关联下载任务 %q 失败: %w", task.Title, err)
				}
				deletedHashes[key] = struct{}{}
			}
			continue
		}
		markDownloadTaskDeletedByID(ctx, s.repo.DB, task)
	}
	return nil
}

func subscriptionDeleteMatchesTask(ctx context.Context, s *SubscriptionService, sub *model.Subscription, task model.DownloadTask) bool {
	if strings.TrimSpace(task.Status) != "" && !downloadTaskBlocksReadd(task.Status) {
		return false
	}
	if strings.TrimSpace(task.SubscriptionID) != "" {
		return task.SubscriptionID == sub.ID
	}
	if strings.TrimSpace(sub.UserID) != "" && strings.TrimSpace(task.UserID) != "" && sub.UserID != task.UserID {
		return false
	}
	baseSavePath := s.subscriptionBaseSavePath(ctx, sub)
	if baseSavePath != "" && task.SavePath != "" && !sameOrChildPath(task.SavePath, baseSavePath) && !sameOrChildPath(baseSavePath, task.SavePath) {
		return false
	}
	query := normalizeAvailabilityComparable(availabilityQuery(subscriptionName(sub), subscriptionFilter(sub)))
	if query == "" {
		return false
	}
	title := normalizeAvailabilityComparable(task.Title)
	if title == "" {
		title = normalizeAvailabilityComparable(publicDownloadTitle(task.URL))
	}
	return title != "" && (strings.Contains(title, query) || strings.Contains(query, title))
}

func downloadTaskInfoHash(task model.DownloadTask) string {
	return torrentURLInfoHash(task.URL)
}

func matchingLiveTorrentHash(task model.DownloadTask, live []QBitTorrent) string {
	if torrent, ok := matchingLiveTorrent(task, live); ok {
		return strings.TrimSpace(torrent.Hash)
	}
	return ""
}

func matchingLiveTorrent(task model.DownloadTask, live []QBitTorrent) (QBitTorrent, bool) {
	for _, torrent := range live {
		if strings.TrimSpace(task.DownloadClientID) != "" && task.DownloadClientID != torrent.ClientID {
			continue
		}
		if strings.TrimSpace(task.ExternalID) != "" && strings.EqualFold(task.ExternalID, torrent.Hash) {
			return torrent, true
		}
	}
	key := downloadTaskIdentityKey(task.Title)
	if key == "" {
		key = downloadTaskIdentityKey(publicDownloadTitle(task.URL))
	}
	if key == "" {
		return QBitTorrent{}, false
	}
	for _, torrent := range live {
		current := downloadTaskIdentityKey(torrent.Name)
		if current == "" {
			continue
		}
		if current == key || strings.Contains(current, key) || strings.Contains(key, current) {
			return torrent, true
		}
	}
	return QBitTorrent{}, false
}

func markDownloadTaskDeletedByID(ctx context.Context, db *gorm.DB, task model.DownloadTask) {
	if db == nil || strings.TrimSpace(task.ID) == "" {
		return
	}
	_ = db.WithContext(ctx).Model(&model.DownloadTask{}).
		Where("id = ?", task.ID).
		Updates(map[string]any{
			"status":   "deleted",
			"progress": task.Progress,
		}).Error
}
