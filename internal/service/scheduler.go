// Package service — periodic scheduled jobs.
//
// SchedulerService runs five recurring background jobs that keep the
// library up-to-date without operator intervention:
//
//	library_scan      every 60 min — re-scan every enabled library so
//	                                  newly-copied files are picked up.
//	subscription_pull every 30 min — re-poll RSS feeds (in addition to
//	                                  the existing SubscriptionService
//	                                  internal timer).
//	download_sync     every 30 s   — refresh the qBittorrent torrent
//	                                  list (already covered by the
//	                                  download poller, kept here as a
//	                                  watchdog).
//	transcode_cleanup every 24 h   — purge HLS transcode artefacts
//	                                  older than 24 h.
//	recycle_purge     every 24 h   — empty the recycle bin of rows
//	                                  soft-deleted more than 30 days
//	                                  ago.
//
// Each job runs at most once at a time (an in-flight run blocks the
// next tick). All work happens on a long-lived background context so
// the operator can keep clicking around the UI while the watchdog runs.
package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// SchedulerService runs the periodic jobs.
type SchedulerService struct {
	log        *zap.Logger
	repo       *repository.Container
	scanner    *ScannerService
	transcoder *TranscoderService
	hub        *Hub
	cacheDir   string

	mu     sync.Mutex
	stopCh chan struct{}
	jobs   []*scheduledJob
}

// scheduledJob is one recurring task.
type scheduledJob struct {
	name     string
	interval time.Duration
	run      func(ctx context.Context) error
	lastRun  time.Time
	lastErr  string
}

// NewSchedulerService is the constructor.
func NewSchedulerService(
	log *zap.Logger,
	repo *repository.Container,
	scanner *ScannerService,
	transcoder *TranscoderService,
	hub *Hub,
	cacheDir string,
) *SchedulerService {
	return &SchedulerService{
		log:        log,
		repo:       repo,
		scanner:    scanner,
		transcoder: transcoder,
		hub:        hub,
		cacheDir:   cacheDir,
		stopCh:     make(chan struct{}),
	}
}

// Start kicks off every job in its own goroutine and returns immediately.
func (s *SchedulerService) Start(ctx context.Context) {
	s.jobs = []*scheduledJob{
		{
			name:     "library_scan",
			interval: 60 * time.Minute,
			run:      s.jobScanLibraries,
		},
		{
			name:     "transcode_cleanup",
			interval: 24 * time.Hour,
			run:      s.jobCleanTranscodeCache,
		},
		{
			name:     "recycle_purge",
			interval: 24 * time.Hour,
			run:      s.jobPurgeRecycleBin,
		},
	}
	for _, j := range s.jobs {
		go s.loop(ctx, j)
	}
}

// Stop signals every job loop to exit on the next tick.
func (s *SchedulerService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.stopCh:
		// already closed
	default:
		close(s.stopCh)
	}
}

// JobStatus is a snapshot suitable for the admin UI.
type JobStatus struct {
	Name     string    `json:"name"`
	Interval string    `json:"interval"`
	LastRun  time.Time `json:"last_run,omitempty"`
	LastErr  string    `json:"last_err,omitempty"`
}

// Status returns the current state of every registered job.
func (s *SchedulerService) Status() []JobStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]JobStatus, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, JobStatus{
			Name:     j.name,
			Interval: j.interval.String(),
			LastRun:  j.lastRun,
			LastErr:  j.lastErr,
		})
	}
	return out
}

// RunNow triggers a single run of the named job synchronously.
func (s *SchedulerService) RunNow(ctx context.Context, name string) error {
	for _, j := range s.jobs {
		if j.name == name {
			return s.runOnce(ctx, j)
		}
	}
	return nil
}

func (s *SchedulerService) loop(ctx context.Context, j *scheduledJob) {
	t := time.NewTicker(j.interval)
	defer t.Stop()
	// Run once shortly after startup so the initial state is fresh.
	first := time.NewTimer(15 * time.Second)
	defer first.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopCh:
			return
		case <-first.C:
		case <-t.C:
		}
		if err := s.runOnce(ctx, j); err != nil {
			s.log.Warn("scheduled job failed",
				zap.String("name", j.name), zap.Error(err))
		}
	}
}

func (s *SchedulerService) runOnce(ctx context.Context, j *scheduledJob) error {
	err := j.run(ctx)
	s.mu.Lock()
	j.lastRun = time.Now()
	if err != nil {
		j.lastErr = err.Error()
	} else {
		j.lastErr = ""
	}
	s.mu.Unlock()
	if s.hub != nil {
		s.hub.Publish("scheduler", map[string]any{
			"name":  j.name,
			"ok":    err == nil,
			"error": j.lastErr,
		})
	}
	return err
}

// jobScanLibraries re-walks every enabled library.
//
// 默认关闭：文件变更由 WatcherService 增量入库，无需周期性全量重扫。
// 仅当用户在设置中显式开启 scan.periodic_enabled 时才执行整库重扫，
// 避免对硬盘的高频反复读取造成损伤（用户明确要求）。
func (s *SchedulerService) jobScanLibraries(ctx context.Context) error {
	if !s.periodicScanEnabled(ctx) {
		return nil
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return err
	}
	for _, l := range libs {
		if !l.Enabled {
			continue
		}
		if _, err := s.scanner.ScanLibrary(ctx, l.ID); err != nil {
			s.log.Warn("scheduled scan failed",
				zap.String("library", l.ID), zap.Error(err))
		}
	}
	return nil
}

// periodicScanEnabled reports whether the operator opted into periodic full
// library re-scans. Defaults to false so the incremental watcher is the only
// thing touching the disk under normal operation.
func (s *SchedulerService) periodicScanEnabled(ctx context.Context) bool {
	if s.repo == nil || s.repo.Setting == nil {
		return false
	}
	v, err := s.repo.Setting.Get(ctx, "scan.periodic_enabled")
	if err != nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

// jobCleanTranscodeCache deletes HLS artefacts older than 24h.
func (s *SchedulerService) jobCleanTranscodeCache(ctx context.Context) error {
	if s.cacheDir == "" {
		return nil
	}
	cutoff := time.Now().Add(-24 * time.Hour)
	return walkAndPrune(s.cacheDir+"/hls", cutoff)
}

// jobPurgeRecycleBin permanently deletes media rows soft-deleted >30 days
// ago. The on-disk file is left untouched (delete is operator-driven).
func (s *SchedulerService) jobPurgeRecycleBin(ctx context.Context) error {
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	res := s.repo.DB.WithContext(ctx).
		Unscoped().
		Where("deleted_at IS NOT NULL AND deleted_at < ?", cutoff).
		Delete(&model.Media{})
	if res.Error != nil && !isMissingTableErr(res.Error) {
		return res.Error
	}
	return nil
}

// isMissingTableErr lets the test harness ignore "no such table" errors
// that show up before AutoMigrate has run.
func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	return err == gorm.ErrInvalidDB
}
