package service

import (
	"context"
	"errors"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (d *DownloadService) PauseDownloadTask(ctx context.Context, taskID string) error {
	return d.controlDownloadTask(ctx, taskID, "paused", func(target downloadTarget, externalID string) error {
		if target.legacyQB {
			return d.qb.Pause(ctx, externalID)
		}
		return target.adapter.Pause(ctx, externalID)
	})
}

func (d *DownloadService) ResumeDownloadTask(ctx context.Context, taskID string) error {
	return d.controlDownloadTask(ctx, taskID, "queued", func(target downloadTarget, externalID string) error {
		if target.legacyQB {
			return d.qb.Resume(ctx, externalID)
		}
		return target.adapter.Resume(ctx, externalID)
	})
}

func (d *DownloadService) controlDownloadTask(ctx context.Context, taskID, status string, operation func(downloadTarget, string) error) error {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return errors.New("task id is required")
	}
	var task model.DownloadTask
	if d == nil || d.repo == nil || d.repo.DB == nil {
		return errors.New("download repository is unavailable")
	}
	if err := d.repo.DB.WithContext(ctx).Where("id = ?", taskID).First(&task).Error; err != nil {
		return err
	}
	clientID, externalID := d.resolveTaskDownloaderIdentity(ctx, &task)
	if externalID == "" {
		return errors.New("download task has no client task id")
	}
	target, err := d.downloadTargetByID(ctx, clientID)
	if err != nil {
		return err
	}
	if err := operation(target, externalID); err != nil {
		return err
	}
	return d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).
		Where("id = ?", task.ID).
		Updates(map[string]any{
			"status":             status,
			"download_client_id": clientID,
			"external_id":        externalID,
			"source":             firstNonEmpty(target.typ, task.Source),
		}).Error
}

func (d *DownloadService) resolveTaskDownloaderIdentity(ctx context.Context, task *model.DownloadTask) (string, string) {
	if task == nil {
		return "", ""
	}
	clientID := strings.TrimSpace(task.DownloadClientID)
	persistedExternalID := strings.TrimSpace(task.ExternalID)
	externalID := persistedExternalID
	if externalID == "" {
		externalID = torrentURLInfoHash(task.URL)
	}
	if clientID != "" && persistedExternalID != "" {
		return clientID, externalID
	}
	live, _ := d.listLiveTorrents(ctx, "")
	for _, torrent := range live {
		if clientID != "" && torrent.ClientID != clientID {
			continue
		}
		if externalID != "" && strings.EqualFold(torrent.Hash, externalID) {
			clientID = firstNonEmpty(clientID, torrent.ClientID)
			return clientID, torrent.Hash
		}
		if downloadTaskMatchesLiveTorrent(*task, torrent) {
			clientID = firstNonEmpty(clientID, torrent.ClientID)
			externalID = firstNonEmpty(externalID, torrent.Hash)
			return clientID, externalID
		}
	}
	if clientID == "" && d.manager != nil {
		var matched []managedDownloadTarget
		for _, target := range d.manager.targets() {
			if strings.TrimSpace(task.Source) == "" || target.client.Type == task.Source {
				matched = append(matched, target)
			}
		}
		if len(matched) == 1 {
			clientID = matched[0].client.ID
		}
	}
	if clientID == "" && d.qb != nil && d.qb.IsConfigured() && (task.Source == "" || task.Source == "qbittorrent") {
		clientID = legacyQBitDownloadClientID
	}
	return clientID, externalID
}
