// Package service — RSS subscriptions for automated downloads.
//
// SubscriptionService periodically polls every Subscription row, fetches
// the configured RSS / Atom feed, and queues new items into the
// DownloadService. Items are deduplicated by GUID stored as a Setting key
// "subscription.<id>.last_guid" so the same episode is never re-queued.
package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// SubscriptionService runs the polling loop.
type SubscriptionService struct {
	cfg            *config.Config
	log            *zap.Logger
	repo           *repository.Container
	downloads      *DownloadService
	site           *SiteService
	resourceImport *ResourceImportService
	scraper        *ScraperService
	hub            *Hub
	notify         *NotifyChannelService
	mu             sync.Mutex
	stop           chan struct{}
	running        bool
}

const (
	defaultSubscriptionPollInterval = 3 * time.Hour
	minSubscriptionPollInterval     = 3 * time.Hour
	subscriptionSchedulerTick       = time.Minute
	subscriptionCatchUpInterval     = time.Minute
)

// NewSubscriptionService is the constructor.
func NewSubscriptionService(cfg *config.Config, log *zap.Logger, repo *repository.Container, downloads *DownloadService, site *SiteService, hub *Hub) *SubscriptionService {
	return &SubscriptionService{
		cfg:       cfg,
		log:       log,
		repo:      repo,
		downloads: downloads,
		site:      site,
		hub:       hub,
	}
}

func (s *SubscriptionService) SetScraper(scraper *ScraperService) {
	s.scraper = scraper
}

func (s *SubscriptionService) SetNotifyChannels(notify *NotifyChannelService) {
	s.notify = notify
}

func (s *SubscriptionService) SetResourceImport(resourceImport *ResourceImportService) {
	s.resourceImport = resourceImport
}

// Start runs the polling loop in the background.
func (s *SubscriptionService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	stop := make(chan struct{})
	s.stop = stop
	s.running = true
	s.mu.Unlock()
	go s.loop(ctx, stop)
}

// Stop shuts the loop down.
func (s *SubscriptionService) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	stop := s.stop
	s.stop = nil
	s.running = false
	s.mu.Unlock()
	close(stop)
}

// Create persists a new subscription.
func (s *SubscriptionService) Create(ctx context.Context, sub *model.Subscription) error {
	if sub == nil || strings.TrimSpace(sub.Name) == "" {
		return errors.New("订阅名称不能为空")
	}
	normalizeSubscriptionDefaults(sub)
	if err := s.ValidateForSave(ctx, sub); err != nil {
		return err
	}
	if duplicate, err := s.resourceImportSubscriptionDuplicate(ctx, sub, ""); err != nil {
		return err
	} else if duplicate {
		return errors.New("相同作品、目标目录和季数的追更订阅已存在")
	}
	if reused, err := s.reuseArchivedResourceImportSubscription(ctx, sub); err != nil {
		return err
	} else if reused {
		return nil
	}
	enabled := sub.Enabled
	if err := s.repo.Subscription.Create(ctx, sub); err != nil {
		return err
	}
	if !enabled {
		if err := s.repo.DB.WithContext(ctx).Model(sub).Update("enabled", false).Error; err != nil {
			return err
		}
		sub.Enabled = false
	}
	return nil
}

func (s *SubscriptionService) Update(ctx context.Context, id string, updates map[string]any) error {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return errors.New("订阅服务不可用")
	}
	return s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sub model.Subscription
		if err := tx.Where("id = ?", strings.TrimSpace(id)).First(&sub).Error; err != nil {
			return err
		}
		if err := tx.Model(&sub).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.Where("id = ?", sub.ID).First(&sub).Error; err != nil {
			return err
		}
		normalizeSubscriptionDefaults(&sub)
		if err := s.ValidateForSave(ctx, &sub); err != nil {
			return err
		}
		if duplicate, err := s.resourceImportSubscriptionDuplicate(ctx, &sub, sub.ID); err != nil {
			return err
		} else if duplicate {
			return errors.New("相同作品、目标目录和季数的追更订阅已存在")
		}
		return tx.Model(&sub).Updates(map[string]any{
			"delivery_mode":         sub.DeliveryMode,
			"feed_url":              sub.FeedURL,
			"resource_source":       sub.ResourceSource,
			"max_imports_per_run":   sub.MaxImportsPerRun,
			"poll_interval_minutes": sub.PollIntervalMinutes,
			"season_number":         sub.SeasonNumber,
			"media_type":            sub.MediaType,
		}).Error
	})
}

func (s *SubscriptionService) reuseArchivedResourceImportSubscription(ctx context.Context, desired *model.Subscription) (bool, error) {
	if !subscriptionUsesResourceImport(desired) || s == nil || s.repo == nil || s.repo.DB == nil {
		return false, nil
	}
	var candidates []model.Subscription
	if err := s.repo.DB.WithContext(ctx).Unscoped().
		Where("delivery_mode = ? AND library_id = ? AND library_root_id = ? AND season_number = ? AND archived_at IS NOT NULL", subscriptionDeliveryResourceImport, desired.LibraryID, desired.LibraryRootID, subscriptionSeasonNumber(desired)).
		Order("archived_at DESC, updated_at DESC").Find(&candidates).Error; err != nil {
		return false, err
	}
	wanted := subscriptionHistoryKey(desired)
	for i := range candidates {
		if subscriptionHistoryKey(&candidates[i]) != wanted {
			continue
		}
		restored, err := s.Restore(ctx, candidates[i].ID)
		if err != nil {
			return false, err
		}
		replacement := *desired
		replacement.Base = restored.Base
		replacement.DeletedAt = gorm.DeletedAt{}
		replacement.ArchivedAt = nil
		replacement.ArchiveReason = ""
		replacement.LastRunAt = nil
		if err := s.repo.DB.WithContext(ctx).Save(&replacement).Error; err != nil {
			return false, err
		}
		*desired = replacement
		return true, nil
	}
	return false, nil
}

func normalizeSubscriptionDefaults(sub *model.Subscription) {
	if sub == nil {
		return
	}
	if strings.TrimSpace(sub.DeliveryMode) == "" {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(sub.FeedURL)), "resource-import://") {
			sub.DeliveryMode = subscriptionDeliveryResourceImport
		} else {
			sub.DeliveryMode = subscriptionDeliveryDownload
		}
	}
	if sub.PollIntervalMinutes <= 0 {
		sub.PollIntervalMinutes = defaultSubscriptionPollIntervalMinutes
	}
	if subscriptionUsesResourceImport(sub) {
		if strings.TrimSpace(sub.ResourceSource) == "" {
			sub.ResourceSource = "default"
		}
		if strings.TrimSpace(sub.FeedURL) == "" {
			sub.FeedURL = "resource-import://default"
		}
		if sub.MaxImportsPerRun <= 0 {
			sub.MaxImportsPerRun = 2
		}
		if sub.SeasonNumber <= 0 {
			sub.SeasonNumber = 1
		}
	}
	if strings.TrimSpace(sub.SearchMode) == "" {
		sub.SearchMode = "keyword"
	}
	if strings.TrimSpace(sub.Resolution) == "" {
		sub.Resolution = "best"
	}
	if strings.TrimSpace(sub.WashPriority) == "" {
		sub.WashPriority = "balanced"
	}
	if sub.Priority == 0 {
		sub.Priority = 50
	}
}

// List returns every subscription rule.
func (s *SubscriptionService) List(ctx context.Context) ([]model.Subscription, error) {
	items, err := s.repo.Subscription.List(ctx)
	if err != nil {
		return nil, err
	}
	return s.attachSubscriptionImportJobs(ctx, items)
}

// Delete removes a subscription.
func (s *SubscriptionService) Delete(ctx context.Context, id string) error {
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&sub).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := s.repo.DB.WithContext(ctx).Unscoped().Where("id = ?", id).First(&sub).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
	}
	if err := s.deleteSubscriptionDownloads(ctx, &sub); err != nil {
		return err
	}
	if s.repo.Setting != nil {
		_ = s.repo.Setting.Delete(ctx, fmt.Sprintf("subscription.%s.seen", id))
	}
	return s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		archivedAt := time.Now()
		if err := tx.Unscoped().Model(&model.Subscription{}).Where("id = ?", id).Updates(map[string]any{
			"enabled":        false,
			"archived_at":    &archivedAt,
			"archive_reason": "手动删除",
		}).Error; err != nil {
			return err
		}
		if sub.DeletedAt.Valid {
			return nil
		}
		return tx.Where("id = ?", id).Delete(&model.Subscription{}).Error
	})
}

// RunNow forces a poll for one subscription, ignoring its schedule. Used
// by the admin UI's "test now" button.
func (s *SubscriptionService) RunNow(ctx context.Context, id string) (int, error) {
	var sub model.Subscription
	if err := s.repo.DB.WithContext(ctx).Where("id = ?", id).First(&sub).Error; err != nil {
		return 0, err
	}
	if sub.ArchivedAt != nil {
		if s.log != nil {
			s.log.Info("subscription run skipped because it is archived",
				zap.String("subscription_id", sub.ID),
				zap.String("subscription", sub.Name),
				zap.String("archive_reason", sub.ArchiveReason))
		}
		return 0, nil
	}
	return s.runOne(ctx, &sub)
}

// loop polls subscription feeds and site-search subscriptions at a conservative
// cadence so tracker APIs are not hammered by every alias keyword.
func (s *SubscriptionService) loop(ctx context.Context, stop <-chan struct{}) {
	defer s.markLoopStopped(stop)
	for {
		timer := time.NewTimer(subscriptionSchedulerTick)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-stop:
			timer.Stop()
			return
		case <-timer.C:
		}
		s.runAll(ctx)
	}
}

func (s *SubscriptionService) markLoopStopped(stop <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop == stop {
		s.stop = nil
		s.running = false
	}
}

func (s *SubscriptionService) pollInterval(ctx context.Context) time.Duration {
	if s == nil || s.repo == nil || s.repo.Setting == nil {
		return defaultSubscriptionPollInterval
	}
	raw, err := s.repo.Setting.Get(ctx, "subscription.interval_seconds")
	if err != nil {
		return defaultSubscriptionPollInterval
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || seconds <= 0 {
		return defaultSubscriptionPollInterval
	}
	interval := time.Duration(seconds) * time.Second
	if interval < minSubscriptionPollInterval {
		return minSubscriptionPollInterval
	}
	return interval
}

func (s *SubscriptionService) runAll(ctx context.Context) {
	subs, err := s.repo.Subscription.List(ctx)
	if err != nil {
		s.log.Warn("subscription list failed", zap.Error(err))
		return
	}
	if s.log != nil {
		s.log.Info("subscription sweep started", zap.Int("count", len(subs)))
	}
	legacyInterval := s.pollInterval(ctx)
	now := time.Now()
	for i := range subs {
		if !subs[i].Enabled {
			continue
		}
		if !subscriptionRunDue(&subs[i], now, legacyInterval, subs[i].CatchUpActive) {
			continue
		}
		stopped, stopErr := s.stopFailedResourceImportSubscription(ctx, &subs[i])
		if stopErr != nil {
			s.log.Warn("subscription failed resource import check failed", zap.String("name", subs[i].Name), zap.Error(stopErr))
			continue
		}
		if stopped {
			continue
		}
		active, activeErr := s.hasActiveResourceImport(ctx, &subs[i])
		if activeErr != nil {
			s.log.Warn("subscription active task check failed", zap.String("name", subs[i].Name), zap.Error(activeErr))
			continue
		}
		if active {
			continue
		}
		if n, err := s.runOne(ctx, &subs[i]); err != nil {
			s.markSubscriptionRun(ctx, &subs[i], now)
			s.log.Warn("subscription run failed",
				zap.String("name", subs[i].Name), zap.Error(err))
			if subscriptionSiteSearchShouldStopOnError(err) {
				s.log.Warn("subscription sweep stopped after upstream failure",
					zap.String("name", subs[i].Name), zap.Error(err))
				return
			}
		} else if n > 0 {
			s.log.Info("subscription queued items",
				zap.String("name", subs[i].Name), zap.Int("count", n))
		}
	}
}

func subscriptionRunDue(sub *model.Subscription, now time.Time, fallback time.Duration, catchUpActive bool) bool {
	if sub == nil || sub.LastRunAt == nil {
		return true
	}
	interval := fallback
	if sub.PollIntervalMinutes > 0 {
		interval = time.Duration(sub.PollIntervalMinutes) * time.Minute
	}
	if interval <= 0 {
		interval = defaultSubscriptionPollInterval
	}
	if catchUpActive && interval > subscriptionCatchUpInterval {
		interval = subscriptionCatchUpInterval
	}
	return !now.Before(sub.LastRunAt.Add(interval))
}

func (s *SubscriptionService) hasActiveResourceImport(ctx context.Context, sub *model.Subscription) (bool, error) {
	if !subscriptionUsesResourceImport(sub) || s == nil || s.repo == nil || s.repo.DB == nil {
		return false, nil
	}
	var count int64
	err := s.repo.DB.WithContext(ctx).Model(&model.ResourceImportJob{}).
		Where("subscription_id = ? AND status NOT IN ?", sub.ID, resourceImportFinalStatuses).
		Count(&count).Error
	return count > 0, err
}

func (s *SubscriptionService) markSubscriptionRun(ctx context.Context, sub *model.Subscription, now time.Time) {
	if s == nil || s.repo == nil || s.repo.DB == nil || sub == nil {
		return
	}
	if err := s.repo.DB.WithContext(ctx).Model(sub).Update("last_run_at", &now).Error; err != nil && s.log != nil {
		s.log.Warn("subscription last_run_at update failed", zap.String("subscription_id", sub.ID), zap.Error(err))
	}
}
