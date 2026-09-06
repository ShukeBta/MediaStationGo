package service

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func (c *Container) warmMediaSearchIndex(ctx context.Context) {
	if c == nil || c.Repo == nil || c.Repo.Media == nil {
		return
	}
	if !mediaSearchWarmupEnabled(ctx, c.Repo) {
		if c.Log != nil {
			c.Log.Info("media search index warmup disabled")
		}
		return
	}
	// 错峰：FTS 正常由 media 表触发器实时维护，回填只是升级或异常后的
	// 兜底。先让登录、首页等关键路径跑起来，再开始后台补索引。
	select {
	case <-ctx.Done():
		return
	case <-time.After(mediaSearchWarmupDelay(ctx, c.Repo)):
	}
	batchSize := mediaSearchWarmupBatchSize(ctx, c.Repo)
	pause := mediaSearchWarmupPause(ctx, c.Repo)
	idle := mediaSearchWarmupIdleInterval(ctx, c.Repo)
	total := int64(0)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		aliases, err := c.Repo.Media.BackfillSearchAliases(ctx, batchSize)
		if err != nil {
			c.Log.Debug("media search index warmup stopped", zap.Error(err))
			return
		}
		n := aliases
		if aliases == 0 {
			indexed, err := c.Repo.Media.BackfillSearchIndex(ctx, batchSize)
			if err != nil {
				c.Log.Debug("media search index warmup stopped", zap.Error(err))
				return
			}
			n = indexed
		}
		if n == 0 {
			if total > 0 {
				c.Log.Info("media search index warmed", zap.Int64("updated", total))
			}
			total = 0
			select {
			case <-ctx.Done():
				return
			case <-time.After(idle):
			}
			continue
		}
		total += n
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}
}

// warmMediaSeriesKeys incrementally repairs grouping keys after a deploy or
// direct metadata edit. It shares the same delayed, low-impact cadence as the
// search warmup and never blocks request handling.
func (c *Container) warmMediaSeriesKeys(ctx context.Context) {
	if c == nil || c.Repo == nil || c.Repo.Media == nil {
		return
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(mediaSearchWarmupDelay(ctx, c.Repo)):
	}
	batchSize := mediaSearchWarmupBatchSize(ctx, c.Repo)
	pause := mediaSearchWarmupPause(ctx, c.Repo)
	idle := mediaSearchWarmupIdleInterval(ctx, c.Repo)
	total := int64(0)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := c.Repo.Media.BackfillSeriesKeys(ctx, batchSize)
		if err != nil {
			if c.Log != nil {
				c.Log.Debug("media series key warmup stopped", zap.Error(err))
			}
			return
		}
		if n == 0 {
			if total > 0 && c.Log != nil {
				c.Log.Info("media series keys warmed", zap.Int64("updated", total))
			}
			total = 0
			select {
			case <-ctx.Done():
				return
			case <-time.After(idle):
			}
			continue
		}
		total += n
		select {
		case <-ctx.Done():
			return
		case <-time.After(pause):
		}
	}
}

func mediaSearchWarmupIdleInterval(ctx context.Context, repo *repository.Container) time.Duration {
	seconds := mediaSearchWarmupIntSetting(ctx, repo, "search.index_warmup_idle_seconds", 60)
	if seconds < 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func mediaSearchWarmupEnabled(ctx context.Context, repo *repository.Container) bool {
	if repo == nil || repo.Setting == nil {
		return true
	}
	value, err := repo.Setting.Get(ctx, "search.index_warmup_enabled")
	if err != nil || strings.TrimSpace(value) == "" {
		return true
	}
	return parseBoolSetting(value, true)
}

func mediaSearchWarmupDelay(ctx context.Context, repo *repository.Container) time.Duration {
	seconds := mediaSearchWarmupIntSetting(ctx, repo, "search.index_warmup_delay_seconds", 120)
	if seconds < 30 {
		seconds = 30
	}
	return time.Duration(seconds) * time.Second
}

func mediaSearchWarmupBatchSize(ctx context.Context, repo *repository.Container) int {
	size := mediaSearchWarmupIntSetting(ctx, repo, "search.index_warmup_batch_size", 100)
	if size < 10 {
		size = 10
	}
	if size > 1000 {
		size = 1000
	}
	return size
}

func mediaSearchWarmupPause(ctx context.Context, repo *repository.Container) time.Duration {
	ms := mediaSearchWarmupIntSetting(ctx, repo, "search.index_warmup_pause_ms", 2000)
	if ms < 250 {
		ms = 250
	}
	return time.Duration(ms) * time.Millisecond
}

func mediaSearchWarmupIntSetting(ctx context.Context, repo *repository.Container, key string, fallback int) int {
	if repo == nil || repo.Setting == nil {
		return fallback
	}
	value, err := repo.Setting.Get(ctx, key)
	if err != nil {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}
