package service

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

// The bounded queue keeps diagnostics off the media response path. It stores
// only middleware-projected metadata; credential-bearing requests never enter it.
type PlayerRequestLogService struct {
	repo    *repository.Container
	log     *zap.Logger
	queue   chan model.PlayerRequestLog
	once    sync.Once
	dropped atomic.Uint64
}

func NewPlayerRequestLogService(repo *repository.Container, log *zap.Logger) *PlayerRequestLogService {
	return &PlayerRequestLogService{repo: repo, log: log, queue: make(chan model.PlayerRequestLog, 1024)}
}

func (s *PlayerRequestLogService) Record(row model.PlayerRequestLog) {
	if s == nil {
		return
	}
	select {
	case s.queue <- row:
	default:
		s.dropped.Add(1)
	}
}

func (s *PlayerRequestLogService) Dropped() uint64 {
	if s == nil {
		return 0
	}
	return s.dropped.Load()
}

func (s *PlayerRequestLogService) Start(ctx context.Context) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		go func() {
			cleanup := time.NewTicker(24 * time.Hour)
			defer cleanup.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case row := <-s.queue:
					writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
					err := s.repo.DB.WithContext(writeCtx).Create(&row).Error
					cancel()
					if err != nil && s.log != nil {
						s.log.Warn("player request log write failed", zap.Error(err))
					}
				case <-cleanup.C:
					_ = s.repo.DB.WithContext(ctx).Unscoped().Where("requested_at < ?", time.Now().AddDate(0, 0, -30)).Delete(&model.PlayerRequestLog{}).Error
				}
			}
		}()
	})
}
