package service

import (
	"context"
	"fmt"
	"time"
)

func (s *PluginService) Run(ctx context.Context, id string) (result PluginRunResult, err error) {
	entry, ok := s.entries[id]
	if !ok {
		return result, ErrPluginNotFound
	}
	state, err := s.beginRun(ctx, entry)
	if err != nil {
		return result, err
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("plugin execution panicked")
		}
		if err == nil {
			err = ctx.Err()
		}
		entry.mu.Lock()
		defer entry.mu.Unlock()
		entry.running = false
		if err != nil {
			entry.lastError = "运行失败，请重试或检查服务日志"
			return
		}
		result.CompletedAt = time.Now().UTC()
		result.DurationMS = time.Since(started).Milliseconds()
		if result.Metrics == nil {
			result.Metrics = []PluginMetric{}
		}
		entry.lastError = ""
		entry.lastRun = clonePluginResult(&result)
	}()
	if err = ctx.Err(); err != nil {
		return result, err
	}
	return entry.plugin.Run(ctx, state.Config)
}

func (s *PluginService) beginRun(ctx context.Context, entry *pluginEntry) (pluginState, error) {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.running {
		return pluginState{}, ErrPluginBusy
	}
	state, err := s.load(ctx, entry)
	if err != nil {
		return state, err
	}
	if !state.Enabled {
		return state, ErrPluginDisabled
	}
	entry.running = true
	return state, nil
}
