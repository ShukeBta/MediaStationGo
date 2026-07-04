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
	cfg       *config.Config
	log       *zap.Logger
	repo      *repository.Container
	downloads *DownloadService
	site      *SiteService
	scraper   *ScraperService
	hub       *Hub
	notify    *NotifyChannelService
	mu        sync.Mutex
	stop      chan struct{}
	running   bool
}

const (
	defaultSubscriptionPollInterval = 3 * time.Hour
	minSubscriptionPollInterval     = 3 * time.Hour
	subscriptionStartupDelay        = defaultSubscriptionPollInterval
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
	if sub.Name == "" || sub.FeedURL == "" {
		return errors.New("name and feed_url required")
	}
	normalizeSubscriptionDefaults(sub)
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

func normalizeSubscriptionDefaults(sub *model.Subscription) {
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
	return s.repo.Subscription.List(ctx)
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
		if err := tx.Unscoped().Model(&model.Subscription{}).Where("id = ?", id).Update("enabled", false).Error; err != nil {
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
	interval := s.pollInterval(ctx)
	delay := subscriptionStartupDelay
	if interval < delay {
		delay = interval
	}
	for {
		timer := time.NewTimer(delay)
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
		// Re-read after every run so changes from the settings page take effect
		// without restarting the service.
		delay = s.pollInterval(ctx)
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
	for i := range subs {
		if !subs[i].Enabled {
			continue
		}
		if n, err := s.runOne(ctx, &subs[i]); err != nil {
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
