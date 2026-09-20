package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

type RuntimeCacheService struct {
	log    *zap.Logger
	client *redis.Client
	prefix string

	mu        sync.RWMutex
	memory    map[string]runtimeCacheItem
	revisions map[string]uint64
	limit     int
}

type runtimeCacheItem struct {
	raw       []byte
	expiresAt time.Time
}

func NewRuntimeCacheService(cfg *config.Config, log *zap.Logger) *RuntimeCacheService {
	c := &RuntimeCacheService{log: log, memory: map[string]runtimeCacheItem{}, revisions: map[string]uint64{}, limit: 2048}
	if cfg == nil {
		return c
	}
	c.prefix = strings.Trim(strings.TrimSpace(cfg.Cache.RedisPrefix), ":")
	if c.prefix == "" {
		c.prefix = "mediastationgo"
	}
	rawURL := strings.TrimSpace(cfg.Cache.RedisURL)
	if rawURL == "" {
		return c
	}
	opts, err := redis.ParseURL(rawURL)
	if err != nil {
		if log != nil {
			log.Warn("redis cache disabled: invalid redis url", zap.Error(err))
		}
		return c
	}
	client := redis.NewClient(opts)
	pingCtx, cancel := context.WithTimeout(context.Background(), 1200*time.Millisecond)
	defer cancel()
	if err := client.Ping(pingCtx).Err(); err != nil {
		if log != nil {
			log.Warn("redis cache unavailable; using in-process cache", zap.Error(err))
		}
		_ = client.Close()
		return c
	}
	c.client = client
	if log != nil {
		log.Info("redis runtime cache enabled with in-process L1", zap.String("addr", opts.Addr), zap.String("prefix", c.prefix))
	}
	return c
}

func (c *RuntimeCacheService) Enabled() bool {
	return c != nil
}

func (c *RuntimeCacheService) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *RuntimeCacheService) GetJSON(ctx context.Context, key string, out any) bool {
	if !c.Enabled() || strings.TrimSpace(key) == "" || out == nil {
		return false
	}
	fullKey := c.key(key)
	if raw, ok := c.getMemory(fullKey); ok {
		return json.Unmarshal(raw, out) == nil
	}
	if c.client != nil {
		raw, err := c.client.Get(ctx, fullKey).Bytes()
		if err == nil {
			if json.Unmarshal(raw, out) != nil {
				return false
			}
			c.setMemory(fullKey, raw, 2*time.Second)
			return true
		}
	}
	return false
}

func (c *RuntimeCacheService) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) {
	if !c.Enabled() || strings.TrimSpace(key) == "" || value == nil || ttl <= 0 {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	fullKey := c.key(key)
	c.setMemory(fullKey, raw, ttl)
	if c.client != nil {
		_ = c.client.Set(ctx, fullKey, raw, ttl).Err()
	}
}

func (c *RuntimeCacheService) DeletePrefix(ctx context.Context, prefix string) {
	if !c.Enabled() || strings.TrimSpace(prefix) == "" {
		return
	}
	if strings.TrimSpace(prefix) == "media:" {
		c.BumpRevision(ctx, "media")
	}
	fullPrefix := c.key(prefix)
	c.deleteMemoryPrefix(fullPrefix)
	if c.client != nil {
		pattern := fullPrefix + "*"
		var cursor uint64
		for {
			keys, next, err := c.client.Scan(ctx, cursor, pattern, 200).Result()
			if err != nil {
				return
			}
			if len(keys) > 0 {
				_ = c.client.Del(ctx, keys...).Err()
			}
			cursor = next
			if cursor == 0 {
				return
			}
		}
	}
}

// Revision identifies an explicitly invalidated cache domain. Redis-backed
// instances share the counter; in-process mode starts empty with its cache.
func (c *RuntimeCacheService) Revision(ctx context.Context, name string) uint64 {
	if !c.Enabled() || strings.TrimSpace(name) == "" {
		return 0
	}
	name = strings.TrimSpace(name)
	if c.client != nil {
		raw, err := c.client.Get(ctx, c.key("runtime:revision:"+name)).Result()
		if err == nil {
			if version, parseErr := strconv.ParseUint(raw, 10, 64); parseErr == nil {
				c.setRevisionMemory(name, version)
				return version
			}
		}
	}
	c.mu.RLock()
	version := c.revisions[name]
	c.mu.RUnlock()
	return version
}

// BumpRevision makes every key derived from the previous version unreachable
// before best-effort physical deletion begins.
func (c *RuntimeCacheService) BumpRevision(ctx context.Context, name string) uint64 {
	if !c.Enabled() || strings.TrimSpace(name) == "" {
		return 0
	}
	name = strings.TrimSpace(name)
	if c.client != nil {
		if version, err := c.client.Incr(ctx, c.key("runtime:revision:"+name)).Result(); err == nil {
			result := uint64(version)
			c.setRevisionMemory(name, result)
			return result
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.revisions == nil {
		c.revisions = map[string]uint64{}
	}
	c.revisions[name]++
	return c.revisions[name]
}

func (c *RuntimeCacheService) setRevisionMemory(name string, version uint64) {
	c.mu.Lock()
	if c.revisions == nil {
		c.revisions = map[string]uint64{}
	}
	c.revisions[name] = version
	c.mu.Unlock()
}

func (c *RuntimeCacheService) key(key string) string {
	key = strings.TrimLeft(strings.TrimSpace(key), ":")
	if c.prefix == "" {
		return key
	}
	return c.prefix + ":" + key
}

func (c *RuntimeCacheService) getMemory(key string) ([]byte, bool) {
	now := time.Now()
	c.mu.RLock()
	item, ok := c.memory[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if now.After(item.expiresAt) {
		c.mu.Lock()
		delete(c.memory, key)
		c.mu.Unlock()
		return nil, false
	}
	return item.raw, true
}

func (c *RuntimeCacheService) setMemory(key string, raw []byte, ttl time.Duration) {
	if ttl <= 0 || len(raw) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.memory) >= c.limit {
		now := time.Now()
		for k, item := range c.memory {
			if now.After(item.expiresAt) || len(c.memory) >= c.limit {
				delete(c.memory, k)
			}
			if len(c.memory) < c.limit {
				break
			}
		}
	}
	c.memory[key] = runtimeCacheItem{raw: append([]byte(nil), raw...), expiresAt: time.Now().Add(ttl)}
}

func (c *RuntimeCacheService) deleteMemoryPrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.memory {
		if strings.HasPrefix(key, prefix) {
			delete(c.memory, key)
		}
	}
}
