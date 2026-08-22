package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// History returns completed/archived subscription rules.
func (s *SubscriptionService) History(ctx context.Context) ([]model.Subscription, error) {
	items, err := s.repo.Subscription.History(ctx)
	if err != nil {
		return nil, err
	}
	if s == nil || s.repo == nil || s.repo.DB == nil || len(items) == 0 {
		return items, nil
	}
	items = groupSubscriptionHistory(items)
	return s.attachSubscriptionImportJobs(ctx, items)
}

// PurgeHistory removes an archived rule and all of its logically equivalent
// historical rows. Import audit rows are removed with the rule; media records
// and cloud files are intentionally untouched.
func (s *SubscriptionService) PurgeHistory(ctx context.Context, id string) error {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return fmt.Errorf("订阅服务不可用")
	}
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).First(&sub).Error; err != nil {
		return err
	}
	if sub.ArchivedAt == nil {
		return fmt.Errorf("只能删除订阅历史记录")
	}
	return s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids, err := matchingSubscriptionHistoryIDs(ctx, tx, &sub)
		if err != nil {
			return err
		}
		if err := tx.Unscoped().Where("subscription_id IN ?", ids).Delete(&model.ResourceImportJob{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Where("id IN ?", ids).Delete(&model.Subscription{}).Error
	})
}

func groupSubscriptionHistory(items []model.Subscription) []model.Subscription {
	grouped := make([]model.Subscription, 0, len(items))
	indexes := map[string]int{}
	for _, item := range items {
		key := subscriptionHistoryKey(&item)
		if index, found := indexes[key]; found {
			grouped[index].HistoryIDs = append(grouped[index].HistoryIDs, item.ID)
			continue
		}
		item.HistoryIDs = []string{item.ID}
		indexes[key] = len(grouped)
		grouped = append(grouped, item)
	}
	return grouped
}

func subscriptionHistoryKey(sub *model.Subscription) string {
	if sub == nil || !subscriptionUsesResourceImport(sub) {
		if sub == nil {
			return ""
		}
		return "id:" + sub.ID
	}
	return strings.Join([]string{
		"resource", sub.LibraryID, sub.LibraryRootID,
		strconv.Itoa(subscriptionSeasonNumber(sub)), resourceImportSubscriptionWorkKey(sub),
	}, "|")
}

func matchingSubscriptionHistoryIDs(ctx context.Context, db *gorm.DB, sub *model.Subscription) ([]string, error) {
	if sub == nil {
		return nil, fmt.Errorf("订阅记录不存在")
	}
	var rows []model.Subscription
	if err := db.WithContext(ctx).Unscoped().Where("archived_at IS NOT NULL").Find(&rows).Error; err != nil {
		return nil, err
	}
	key := subscriptionHistoryKey(sub)
	ids := make([]string, 0, len(rows))
	for i := range rows {
		if subscriptionHistoryKey(&rows[i]) == key {
			ids = append(ids, rows[i].ID)
		}
	}
	if len(ids) == 0 {
		return []string{sub.ID}, nil
	}
	return ids, nil
}

func (s *SubscriptionService) attachSubscriptionImportJobs(ctx context.Context, items []model.Subscription) ([]model.Subscription, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil || len(items) == 0 {
		return items, nil
	}
	ids := make([]string, 0, len(items))
	groupIndexes := map[string]int{}
	for i := range items {
		historyIDs := items[i].HistoryIDs
		if len(historyIDs) == 0 {
			historyIDs = []string{items[i].ID}
		}
		for _, id := range historyIDs {
			ids = append(ids, id)
			groupIndexes[id] = i
		}
	}
	var jobs []model.ResourceImportJob
	if err := s.repo.DB.WithContext(ctx).Where("subscription_id IN ? AND subscription_follow = ?", ids, true).
		Order("created_at DESC, attempt DESC").Find(&jobs).Error; err != nil {
		return nil, err
	}
	bySubscription := map[int][]model.SubscriptionImportJob{}
	for _, job := range jobs {
		index, found := groupIndexes[job.SubscriptionID]
		if !found {
			continue
		}
		selected, moved, verified, scanAdded, blockReason := subscriptionImportAuditDetails(job.ResultJSON)
		bySubscription[index] = append(bySubscription[index], model.SubscriptionImportJob{
			ID: job.ID, RetryOfJobID: job.RetryOfJobID, Attempt: job.Attempt,
			CandidateTitle: job.CandidateTitle, CandidateSource: job.CandidateSource,
			CandidateGranularity: job.TitleClass, SelectedEpisodes: selected,
			MovedEpisodes: moved, VerifiedEpisodes: verified, ScanAdded: scanAdded,
			BlockReason: blockReason, Status: job.Status, Stage: job.Stage,
			Outcome: job.Outcome, Error: job.PublicError,
			CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt, FinishedAt: job.FinishedAt,
		})
	}
	for i := range items {
		items[i].ImportJobs = bySubscription[i]
	}
	return items, nil
}

func subscriptionImportAuditDetails(raw string) (selected, moved, verified []int, scanAdded int, blockReason string) {
	var result struct {
		SubscriptionFollow struct {
			SelectedEpisodes []int `json:"selected_episodes"`
			MovedEpisodes    []int `json:"moved_episodes"`
			VerifiedEpisodes []int `json:"verified_episodes"`
			ScanAdded        int   `json:"scan_added"`
			SourceBlock      struct {
				Reason string `json:"reason"`
			} `json:"source_block"`
		} `json:"subscription_follow"`
	}
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &result) != nil {
		return nil, nil, nil, 0, ""
	}
	return uniqueSortedPositiveInts(result.SubscriptionFollow.SelectedEpisodes),
		uniqueSortedPositiveInts(result.SubscriptionFollow.MovedEpisodes),
		uniqueSortedPositiveInts(result.SubscriptionFollow.VerifiedEpisodes),
		result.SubscriptionFollow.ScanAdded,
		strings.TrimSpace(result.SubscriptionFollow.SourceBlock.Reason)
}

// Restore moves an archived subscription back to the active management list.
// It also clears the per-subscription seen state so an unfinished historical
// rule can match resources again when it is run next.
func (s *SubscriptionService) Restore(ctx context.Context, id string) (*model.Subscription, error) {
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).First(&sub).Error; err != nil {
		return nil, err
	}
	if err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := mergeSubscriptionHistoryRecords(ctx, tx, &sub); err != nil {
			return err
		}
		return tx.Unscoped().Model(&model.Subscription{}).
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
			}).Error
	}); err != nil {
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

func mergeSubscriptionHistoryRecords(ctx context.Context, tx *gorm.DB, target *model.Subscription) error {
	ids, err := matchingSubscriptionHistoryIDs(ctx, tx, target)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if id == target.ID {
			continue
		}
		if err := tx.Model(&model.ResourceImportJob{}).Where("subscription_id = ?", id).Update("subscription_id", target.ID).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("id = ?", id).Delete(&model.Subscription{}).Error; err != nil {
			return err
		}
	}
	return nil
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
