package service

import (
	"context"
	"database/sql"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"
)

type episodeTraceKey struct{}
type episodeTrace struct {
	mu     sync.Mutex
	stages map[string]float64
}

// BeginEpisodeTrace is opt-in at the server, never by an untrusted request.
// No user IDs, query strings, tokens, media paths or response bodies are logged.
// Pool deltas are process-wide overlapping-window observations, NOT attributed
// request wait time. SQL stages include driver wait/decoding, not just SQL CPU.
func (e *EmbyService) BeginEpisodeTrace(ctx context.Context) (context.Context, func(int, int)) {
	if os.Getenv("MEDIASTATION_DIAGNOSTICS_EPISODES") != "1" {
		return ctx, func(int, int) {}
	}
	trace := &episodeTrace{stages: make(map[string]float64)}
	ctx = context.WithValue(ctx, episodeTraceKey{}, trace)
	start := time.Now()
	var pool *sql.DB
	var before sql.DBStats
	if e.repo != nil && e.repo.DB != nil {
		if db, err := e.repo.DB.DB(); err == nil {
			pool, before = db, db.Stats()
		}
	}
	return ctx, func(status, bytes int) {
		trace.mu.Lock()
		defer trace.mu.Unlock()
		fields := []zap.Field{
			zap.Duration("total", time.Since(start)),
			zap.Int("status", status), zap.Int("response_bytes", bytes),
			zap.Any("stage_ms", trace.stages),
			zap.Bool("context_cancelled", ctx.Err() != nil),
		}
		if pool != nil {
			after := pool.Stats()
			fields = append(fields,
				zap.Int64("pool_window_wait_count_delta", after.WaitCount-before.WaitCount),
				zap.Duration("pool_window_wait_duration_delta", after.WaitDuration-before.WaitDuration),
				zap.Int("pool_in_use_end", after.InUse), zap.Int("pool_max_open", after.MaxOpenConnections))
		}
		e.log.Info("emby episode timing", fields...)
	}
}

// MeasureEpisodeStage is a no-op outside an enabled Episodes request.
// Stages are sequential; the handler's service_total is inclusive of service
// stages and must not be added to them.
func MeasureEpisodeStage(ctx context.Context, name string) func() {
	trace, ok := ctx.Value(episodeTraceKey{}).(*episodeTrace)
	if !ok {
		return func() {}
	}
	start := time.Now()
	return func() {
		trace.mu.Lock()
		trace.stages[name] += float64(time.Since(start)) / float64(time.Millisecond)
		trace.mu.Unlock()
	}
}
