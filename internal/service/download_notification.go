package service

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (d *DownloadService) notifyDownloadComplete(ctx context.Context, torrent QBitTorrent, task *model.DownloadTask) {
	if d == nil || d.notify == nil {
		return
	}
	if d.completedTorrentNotified(ctx, torrent) {
		return
	}
	d.markCompletedTorrentNotified(ctx, torrent)
	body, data := downloadCompleteNotificationPayload(torrent, task)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		d.notify.BroadcastEvent(ctx, NotifyEvent{
			Type:    EventDownloadComplete,
			Title:   "MediaStationGo 下载完成",
			Message: body,
			Data:    data,
		})
	}()
}

func downloadCompleteNotificationPayload(torrent QBitTorrent, task *model.DownloadTask) (string, map[string]interface{}) {
	name := downloadCompleteNotificationName(torrent, task)
	body := "任务：" + name
	data := downloadCompleteNotificationData(torrent, task)
	return body, data
}

func downloadCompleteNotificationName(torrent QBitTorrent, task *model.DownloadTask) string {
	name := strings.TrimSpace(torrent.Name)
	if name == "" {
		name = strings.TrimSpace(filepath.Base(torrent.ContentPath))
	}
	if task != nil && strings.TrimSpace(task.Title) != "" {
		name = strings.TrimSpace(task.Title)
	}
	if name == "" {
		name = "下载任务"
	}
	return name
}

func downloadCompleteNotificationData(torrent QBitTorrent, task *model.DownloadTask) map[string]interface{} {
	data := map[string]interface{}{}
	if rt := strings.TrimSpace(torrent.Name); rt != "" {
		data["resource_title"] = rt
	}
	if task == nil {
		return data
	}
	addTrimmedString(data, "poster_url", task.PosterURL)
	addTrimmedString(data, "backdrop_url", task.BackdropURL)
	addTrimmedString(data, "media_type", task.MediaType)
	addTrimmedString(data, "media_category", task.MediaCategory)
	addTrimmedString(data, "title", task.Title)
	addTrimmedString(data, "overview", task.Overview)
	addTrimmedString(data, "original_title", task.OriginalName)
	addTrimmedString(data, "original_language", task.OriginalLanguage)
	if task.Year > 0 {
		data["year"] = task.Year
	}
	if task.Rating > 0 {
		data["rating"] = task.Rating
	}
	addTrimmedString(data, "genres", task.Genres)
	return data
}

func addTrimmedString(data map[string]interface{}, key, value string) {
	if strings.TrimSpace(value) != "" {
		data[key] = value
	}
}
