package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func (s *SubscriptionService) stopResourceImportSubscriptionAfterFailure(ctx context.Context, job model.ResourceImportJob) error {
	if s == nil || s.repo == nil || s.repo.DB == nil || !job.SubscriptionFollow || strings.TrimSpace(job.SubscriptionID) == "" {
		return nil
	}
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", job.SubscriptionID).First(&sub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if !sub.Enabled || sub.ArchivedAt != nil {
		return nil
	}
	result := s.repo.DB.WithContext(ctx).Model(&model.Subscription{}).
		Where("id = ? AND enabled = ? AND archived_at IS NULL", sub.ID, true).
		Updates(map[string]any{"enabled": false, "catch_up_active": false})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	sub.Enabled = false
	sub.CatchUpActive = false
	s.notifyResourceImportSubscriptionStopped(&sub, job)
	if s.hub != nil {
		s.hub.Publish("subscription", map[string]any{
			"id": sub.ID, "name": sub.Name, "enabled": false,
			"reason": "自动追更任务失败，已停止",
		})
	}
	return nil
}

func (s *SubscriptionService) stopFailedResourceImportSubscription(ctx context.Context, sub *model.Subscription) (bool, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil || sub == nil || !subscriptionUsesResourceImport(sub) {
		return false, nil
	}
	var job model.ResourceImportJob
	err := s.repo.DB.WithContext(ctx).
		Where("subscription_id = ? AND subscription_follow = ?", sub.ID, true).
		Order("created_at DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if job.Status != ResourceImportStatusFailed {
		return false, nil
	}
	if job.FinishedAt != nil && sub.LastRunAt != nil && !job.FinishedAt.After(*sub.LastRunAt) {
		return false, nil
	}
	if err := s.stopResourceImportSubscriptionAfterFailure(ctx, job); err != nil {
		return false, err
	}
	return true, nil
}

func (s *SubscriptionService) notifyResourceImportSubscriptionCompleted(ctx context.Context, job model.ResourceImportJob) error {
	if s == nil || s.repo == nil || s.repo.DB == nil || !job.SubscriptionFollow || strings.TrimSpace(job.SubscriptionID) == "" {
		return nil
	}
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", job.SubscriptionID).First(&sub).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	s.notifyResourceImportSubscriptionCompletedEvent(&sub, job)
	return nil
}

func (s *SubscriptionService) notifyResourceImportSubscriptionStopped(sub *model.Subscription, job model.ResourceImportJob) {
	if s == nil || s.notify == nil || sub == nil {
		return
	}
	event := resourceImportSubscriptionStoppedNotification(sub, job)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.notify.BroadcastEvent(ctx, event)
	}()
}

func resourceImportSubscriptionStoppedNotification(sub *model.Subscription, job model.ResourceImportJob) NotifyEvent {
	reason := strings.TrimSpace(job.PublicError)
	if reason == "" {
		reason = strings.TrimSpace(job.Error)
	}
	if reason == "" {
		reason = "自动追更任务失败"
	}
	resource := strings.TrimSpace(job.CandidateTitle)
	body := fmt.Sprintf("剧集：%s\n失败集数：%s\n已停止自动追更\n原因：%s", sub.Name, subscriptionNotificationEpisodes(subscriptionResourceImportJobEpisodes(sub, job)), reason)
	if resource != "" {
		body += "\n资源：" + resource
	}
	body += "\n处理：修复问题后请从任务历史手动重试，再重新启用订阅。"
	data := map[string]interface{}{"title": sub.Name, "resource_title": resource}
	if strings.TrimSpace(sub.PosterURL) != "" {
		data["poster_url"] = sub.PosterURL
	}
	return NotifyEvent{Type: EventSubscriptionStopped, Title: "MediaStationGo 自动追更已停止", Message: body, Data: data}
}

func (s *SubscriptionService) notifyResourceImportSubscriptionCompletedEvent(sub *model.Subscription, job model.ResourceImportJob) {
	if s == nil || s.notify == nil || sub == nil {
		return
	}
	event := resourceImportSubscriptionCompletedNotification(sub, job)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		s.notify.BroadcastEvent(ctx, event)
	}()
}

func resourceImportSubscriptionCompletedNotification(sub *model.Subscription, job model.ResourceImportJob) NotifyEvent {
	episodes := subscriptionResourceImportJobEpisodes(sub, job)
	body := fmt.Sprintf("剧集：%s\n入库集数：%s\n状态：自动追更已成功入库。", sub.Name, subscriptionNotificationEpisodes(episodes))
	if job.Status == ResourceImportStatusCompletedWithWarning {
		body = fmt.Sprintf("剧集：%s\n入库集数：%s\n状态：自动追更已入库完成，但有警告。", sub.Name, subscriptionNotificationEpisodes(episodes))
	}
	data := map[string]interface{}{"title": sub.Name, "resource_title": strings.TrimSpace(job.CandidateTitle), "episodes": subscriptionNotificationEpisodes(episodes)}
	if strings.TrimSpace(sub.PosterURL) != "" {
		data["poster_url"] = sub.PosterURL
	}
	return NotifyEvent{Type: EventSubscriptionCompleted, Title: "MediaStationGo 自动追更入库完成", Message: body, Data: data}
}

func subscriptionResourceImportJobEpisodes(sub *model.Subscription, job model.ResourceImportJob) []int {
	selected, moved, verified, _ := resourceImportSubscriptionProjection(job.ResultJSON)
	for _, episodes := range [][]int{verified, moved, selected} {
		if len(episodes) > 0 {
			return episodes
		}
	}
	return episodeNumbersFromRefs(subscriptionCandidateEpisodeRefs(sub, job.CandidateTitle), job.SeasonNumber)
}
