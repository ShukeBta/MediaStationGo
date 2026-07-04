package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// History returns completed/archived subscription rules.
func (s *SubscriptionService) History(ctx context.Context) ([]model.Subscription, error) {
	return s.repo.Subscription.History(ctx)
}

// Restore moves an archived subscription back to the active management list.
// It also clears the per-subscription seen state so an unfinished historical
// rule can match resources again when it is run next.
func (s *SubscriptionService) Restore(ctx context.Context, id string) (*model.Subscription, error) {
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).First(&sub).Error; err != nil {
		return nil, err
	}
	if err := s.repo.DB.WithContext(ctx).Unscoped().Model(&model.Subscription{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"enabled":        true,
			"archived_at":    nil,
			"archive_reason": "",
			"deleted_at":     nil,
			// 重置为 0:此前可能被 feed 低估并锁死(updateSubscriptionTotalEpisodes
			// 只增不减,resolveSubscriptionTotalEpisodes 见 >0 即不再回查元数据)。
			// 归零后下次 run 会从 TMDb/豆瓣等权威源重算真实总集数,避免恢复后
			// 因"误判已无缺集"而不再搜索资源。
			"total_episodes": 0,
		}).Error; err != nil {
		return nil, err
	}
	if s.repo.Setting != nil {
		_ = s.repo.Setting.Delete(ctx, fmt.Sprintf("subscription.%s.seen", id))
	}
	var restored model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&restored).Error; err != nil {
		return nil, err
	}
	return &restored, nil
}

func (s *SubscriptionService) archiveCompletedSubscription(ctx context.Context, sub *model.Subscription, availability LocalAvailability) error {
	if s == nil || s.repo == nil || s.repo.Subscription == nil || sub == nil {
		return nil
	}
	if !subscriptionShouldArchive(sub, availability) {
		return nil
	}
	now := time.Now()
	reason := subscriptionArchiveReason(sub, availability)
	if err := s.repo.Subscription.Archive(ctx, sub.ID, reason, now); err != nil {
		return err
	}
	sub.Enabled = false
	sub.ArchivedAt = &now
	sub.ArchiveReason = reason
	if s.log != nil {
		s.log.Info("subscription completed, moved to history",
			zap.String("id", sub.ID),
			zap.String("name", sub.Name),
			zap.String("reason", reason))
	}
	if s.hub != nil {
		s.hub.Publish("subscription", map[string]any{
			"id":       sub.ID,
			"name":     sub.Name,
			"archived": true,
			"reason":   reason,
		})
	}
	return nil
}

func subscriptionShouldArchive(sub *model.Subscription, availability LocalAvailability) bool {
	if sub == nil || subscriptionAllowsWash(sub) || sub.ArchivedAt != nil {
		return false
	}
	mediaType := normalizeMediaType(sub.MediaType, sub.Name+" "+sub.Filter, "")
	seriesLike := isSubscriptionSeriesType(mediaType) || len(availability.ExistingEpisodeKeys) > 0 || len(availability.MissingEpisodeKeys) > 0
	if !seriesLike {
		return availability.InLibrary || availability.LocalMediaCount > 0 || availability.DownloadedEpisodes > 0
	}
	total := trustedSeriesArchiveTotal(sub, availability)
	if availability.HasSeriesPack {
		if len(availability.ExistingEpisodeKeys) == 0 {
			return true
		}
		return total > 0 && availability.DownloadedEpisodes >= total && len(availability.MissingEpisodes) == 0
	}
	if total > 0 {
		return availability.DownloadedEpisodes >= total && len(availability.MissingEpisodes) == 0
	}
	return subscriptionLooksSingleEpisode(sub) && availability.DownloadedEpisodes > 0
}

func trustedSeriesArchiveTotal(sub *model.Subscription, availability LocalAvailability) int {
	total := 0
	if sub != nil {
		total = sub.TotalEpisodes
	}
	if total <= 0 {
		total = availability.TotalEpisodes
	}
	if maxEpisode := maxAvailabilityEpisode(availability.ExistingEpisodeKeys); total > 0 && maxEpisode > total {
		return 0
	}
	return total
}

func maxAvailabilityEpisode(keys map[string]struct{}) int {
	maxEpisode := 0
	for key := range keys {
		var season, episode int
		if _, err := fmt.Sscanf(key, "%02dE%03d", &season, &episode); err == nil && episode > maxEpisode {
			maxEpisode = episode
		}
	}
	return maxEpisode
}

func subscriptionArchiveReason(sub *model.Subscription, availability LocalAvailability) string {
	if subscriptionAllowsWash(sub) {
		return ""
	}
	if availability.HasSeriesPack {
		return "整季资源已加入下载/入库"
	}
	if availability.TotalEpisodes > 0 {
		return fmt.Sprintf("订阅完成：%d/%d", availability.DownloadedEpisodes, availability.TotalEpisodes)
	}
	if availability.DownloadedEpisodes > 0 {
		return "单集订阅已加入下载/入库"
	}
	return "订阅媒体已加入下载/入库"
}

func subscriptionLooksSingleEpisode(sub *model.Subscription) bool {
	if sub == nil {
		return false
	}
	for _, value := range []string{sub.Name, sub.Filter} {
		_, episode := ParseEpisode(value)
		if episode > 0 {
			return true
		}
	}
	return false
}
