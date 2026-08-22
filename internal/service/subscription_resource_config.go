package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const (
	subscriptionDeliveryDownload             = "download"
	subscriptionDeliveryResourceImport       = "resource_import"
	maxSubscriptionImportsPerRun             = 5
	defaultSubscriptionPollIntervalMinutes   = 180
	minSubscriptionPollIntervalMinutes       = 5
	maxSubscriptionPollIntervalMinutes       = 24 * 60
	resourceImportVerificationReservationTTL = 10 * time.Minute
)

func subscriptionUsesResourceImport(sub *model.Subscription) bool {
	if sub == nil {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(sub.DeliveryMode))
	if mode == subscriptionDeliveryResourceImport {
		return true
	}
	return mode == "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(sub.FeedURL)), "resource-import://")
}

func subscriptionSeasonNumber(sub *model.Subscription) int {
	if sub != nil && sub.SeasonNumber > 0 {
		return sub.SeasonNumber
	}
	return 1
}

func (s *SubscriptionService) ValidateForSave(ctx context.Context, sub *model.Subscription) error {
	if sub == nil {
		return errors.New("订阅参数不能为空")
	}
	if sub.PollIntervalMinutes != 0 && (sub.PollIntervalMinutes < minSubscriptionPollIntervalMinutes || sub.PollIntervalMinutes > maxSubscriptionPollIntervalMinutes) {
		return errors.New("自动追更扫描频率必须在 5 到 1440 分钟之间")
	}
	switch strings.ToLower(strings.TrimSpace(sub.DeliveryMode)) {
	case subscriptionDeliveryDownload:
		if strings.TrimSpace(sub.FeedURL) == "" {
			return errors.New("RSS / PT 订阅必须填写订阅地址")
		}
		return nil
	case subscriptionDeliveryResourceImport:
		return s.validateResourceImportSubscription(ctx, sub)
	default:
		return errors.New("不支持的订阅投递模式")
	}
}

func (s *SubscriptionService) validateResourceImportSubscription(ctx context.Context, sub *model.Subscription) error {
	if s == nil || s.resourceImport == nil {
		return errors.New("网盘入库服务不可用")
	}
	if s.repo == nil || s.repo.Library == nil {
		return errors.New("媒体库服务不可用")
	}
	source := strings.ToLower(strings.TrimSpace(sub.ResourceSource))
	if source != "default" && source != "pansou" && source != "bt4g" {
		return errors.New("自动追更搜索源无效")
	}
	if sub.MaxImportsPerRun < 1 || sub.MaxImportsPerRun > maxSubscriptionImportsPerRun {
		return errors.New("每轮入库数量必须在 1 到 5 之间")
	}
	if sub.SeasonNumber < 1 {
		return errors.New("季数必须大于 0")
	}
	library, err := s.repo.Library.FindByID(ctx, strings.TrimSpace(sub.LibraryID))
	if err != nil {
		return err
	}
	if library == nil || !library.Enabled {
		return errors.New("目标媒体库不存在或已停用")
	}
	root, err := s.repo.Library.FindRootByID(ctx, library.ID, strings.TrimSpace(sub.LibraryRootID))
	if err != nil {
		return err
	}
	if root == nil || !root.Enabled {
		return errors.New("目标入库目录不存在或已停用")
	}
	if _, err := resourceRootOpenListPath(root.Path); err != nil {
		return err
	}
	mediaType := normalizeMediaType(firstNonEmpty(sub.MediaType, library.Type), sub.Name+" "+sub.Filter, "")
	if !isSubscriptionSeriesType(mediaType) {
		return errors.New("自动追更只支持电视剧、动漫或综艺")
	}
	if strings.TrimSpace(sub.MediaType) == "" {
		sub.MediaType = mediaType
	}
	return nil
}

func (s *SubscriptionService) resourceImportSubscriptionDuplicate(ctx context.Context, sub *model.Subscription, exceptID string) (bool, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil || sub == nil || !subscriptionUsesResourceImport(sub) {
		return false, nil
	}
	var rows []model.Subscription
	query := s.repo.DB.WithContext(ctx).Unscoped().
		Where("delivery_mode = ? AND library_id = ? AND library_root_id = ? AND season_number = ?", subscriptionDeliveryResourceImport, sub.LibraryID, sub.LibraryRootID, subscriptionSeasonNumber(sub)).
		Where("archived_at IS NULL AND deleted_at IS NULL")
	if strings.TrimSpace(exceptID) != "" {
		query = query.Where("id <> ?", strings.TrimSpace(exceptID))
	}
	if err := query.Find(&rows).Error; err != nil {
		return false, err
	}
	wanted := map[string]struct{}{}
	for _, value := range subscriptionTitleMatchQueries(sub) {
		if key := normalizeAvailabilityComparable(value); key != "" {
			wanted[key] = struct{}{}
		}
	}
	for i := range rows {
		for _, value := range subscriptionTitleMatchQueries(&rows[i]) {
			if _, ok := wanted[normalizeAvailabilityComparable(value)]; ok {
				return true, nil
			}
		}
	}
	return false, nil
}
