package main

import (
	"context"
	"database/sql"
	"runtime"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func applyCPUThreadLimit(cfg *config.Config, logger *zap.Logger) {
	if cfg == nil || cfg.App.MaxCPUThreads < 1 {
		return
	}
	prev := runtime.GOMAXPROCS(cfg.App.MaxCPUThreads)
	if logger != nil {
		logger.Info("runtime CPU thread limit applied",
			zap.Int("max_cpu_threads", cfg.App.MaxCPUThreads),
			zap.Int("previous", prev))
	}
}

func waitForDatabase(db interface{ DB() (*sql.DB, error) }, logger *zap.Logger) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 1; attempt <= 30; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = sqlDB.PingContext(ctx)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if logger != nil {
			logger.Warn("database not ready; retrying", zap.Int("attempt", attempt), zap.Error(err))
		}
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
	}
	return lastErr
}
