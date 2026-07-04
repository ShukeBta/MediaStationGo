package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (d *DownloadService) linkExistingDownloadTaskToSubscription(ctx context.Context, task *model.DownloadTask, req downloadAddRequest) {
	subscriptionID := strings.TrimSpace(req.meta.SubscriptionID)
	if d == nil || d.repo == nil || d.repo.DB == nil || task == nil || subscriptionID == "" || strings.TrimSpace(task.ID) == "" {
		return
	}
	updates := map[string]any{}
	if strings.TrimSpace(task.SubscriptionID) == "" {
		updates["subscription_id"] = subscriptionID
		task.SubscriptionID = subscriptionID
	}
	if strings.TrimSpace(task.MediaType) == "" && strings.TrimSpace(req.meta.MediaType) != "" {
		updates["media_type"] = req.meta.MediaType
		task.MediaType = req.meta.MediaType
	}
	if strings.TrimSpace(task.MediaCategory) == "" && strings.TrimSpace(req.meta.MediaCategory) != "" {
		updates["media_category"] = req.meta.MediaCategory
		task.MediaCategory = req.meta.MediaCategory
	}
	if strings.TrimSpace(task.PosterURL) == "" && strings.TrimSpace(req.meta.PosterURL) != "" {
		updates["poster_url"] = req.meta.PosterURL
		task.PosterURL = req.meta.PosterURL
	}
	if strings.TrimSpace(task.BackdropURL) == "" && strings.TrimSpace(req.meta.BackdropURL) != "" {
		updates["backdrop_url"] = req.meta.BackdropURL
		task.BackdropURL = req.meta.BackdropURL
	}
	if strings.TrimSpace(task.Overview) == "" && strings.TrimSpace(req.meta.Overview) != "" {
		updates["overview"] = req.meta.Overview
		task.Overview = req.meta.Overview
	}
	if !task.AllowExistingLibrary && req.meta.AllowExistingLibrary {
		updates["allow_existing_library"] = true
		task.AllowExistingLibrary = true
	}
	if len(updates) == 0 {
		return
	}
	_ = d.repo.DB.WithContext(ctx).Model(&model.DownloadTask{}).Where("id = ?", task.ID).Updates(updates).Error
}
