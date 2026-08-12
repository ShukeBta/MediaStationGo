package service

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// DiscoverSectionCache keeps the last good discover rail in memory so a slow
// upstream provider does not turn a populated page into empty rows.
type DiscoverSectionCache struct {
	ttl     time.Duration
	mu      sync.RWMutex
	entries map[string]discoverSectionCacheEntry
}

type discoverSectionCacheEntry struct {
	items    []ExternalMediaResult
	storedAt time.Time
}

func NewDiscoverSectionCache(ttl time.Duration) *DiscoverSectionCache {
	if ttl <= 0 {
		ttl = 6 * time.Hour
	}
	return &DiscoverSectionCache{
		ttl:     ttl,
		entries: map[string]discoverSectionCacheEntry{},
	}
}

func (d *DiscoverService) RememberSection(key string, page int, items []ExternalMediaResult) {
	if d == nil || d.sectionCache == nil || len(items) == 0 {
		return
	}
	d.sectionCache.Set(key, page, items)
}

func (d *DiscoverService) CachedSection(key string, page int) ([]ExternalMediaResult, bool) {
	if d == nil || d.sectionCache == nil {
		return nil, false
	}
	return d.sectionCache.Get(key, page)
}

// CachedSectionLastGood returns the last successful section result even when
// its normal freshness TTL has elapsed. Background-refresh-only sections use
// this to keep serving the previous result while an upstream is unavailable.
func (d *DiscoverService) CachedSectionLastGood(key string, page int) ([]ExternalMediaResult, bool) {
	if d == nil || d.sectionCache == nil {
		return nil, false
	}
	return d.sectionCache.GetLastGood(key, page)
}

func (d *DiscoverService) ForgetSection(key string) {
	if d == nil || d.sectionCache == nil {
		return
	}
	d.sectionCache.DeleteSection(key)
}

func (c *DiscoverSectionCache) Set(key string, page int, items []ExternalMediaResult) {
	if c == nil || key == "" || page < 1 || len(items) == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[discoverSectionCacheKey(key, page)] = discoverSectionCacheEntry{
		items:    cloneExternalMediaResults(items),
		storedAt: time.Now(),
	}
}

func (c *DiscoverSectionCache) Get(key string, page int) ([]ExternalMediaResult, bool) {
	return c.get(key, page, false)
}

func (c *DiscoverSectionCache) GetLastGood(key string, page int) ([]ExternalMediaResult, bool) {
	return c.get(key, page, true)
}

func (c *DiscoverSectionCache) get(key string, page int, allowExpired bool) ([]ExternalMediaResult, bool) {
	if c == nil || key == "" || page < 1 {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[discoverSectionCacheKey(key, page)]
	c.mu.RUnlock()
	if !ok || (!allowExpired && time.Since(entry.storedAt) > c.ttl) || len(entry.items) == 0 {
		return nil, false
	}
	return cloneExternalMediaResults(entry.items), true
}

func (c *DiscoverSectionCache) DeleteSection(key string) {
	if c == nil || strings.TrimSpace(key) == "" {
		return
	}
	prefix := strings.TrimSpace(key) + ":"
	c.mu.Lock()
	defer c.mu.Unlock()
	for cacheKey := range c.entries {
		if strings.HasPrefix(cacheKey, prefix) {
			delete(c.entries, cacheKey)
		}
	}
}

func discoverSectionCacheKey(key string, page int) string {
	return fmt.Sprintf("%s:%d", key, page)
}

func cloneExternalMediaResults(items []ExternalMediaResult) []ExternalMediaResult {
	out := make([]ExternalMediaResult, len(items))
	for i, item := range items {
		out[i] = item
		out[i].SubscribeAliases = cloneStrings(item.SubscribeAliases)
		out[i].MissingEpisodes = cloneInts(item.MissingEpisodes)
		out[i].PreviewImages = cloneStrings(item.PreviewImages)
		out[i].Languages = cloneStrings(item.Languages)
		out[i].Countries = cloneStrings(item.Countries)
		out[i].Genres = cloneStrings(item.Genres)
		out[i].Actors = cloneStrings(item.Actors)
		out[i].Directors = cloneStrings(item.Directors)
		out[i].Writers = cloneStrings(item.Writers)
		out[i].Aliases = cloneStrings(item.Aliases)
		out[i].People = clonePersonMetadata(item.People)
	}
	return out
}

func clonePersonMetadata(items []PersonMetadata) []PersonMetadata {
	if len(items) == 0 {
		return nil
	}
	out := make([]PersonMetadata, len(items))
	copy(out, items)
	return out
}

func cloneStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, len(items))
	copy(out, items)
	return out
}

func cloneInts(items []int) []int {
	if len(items) == 0 {
		return nil
	}
	out := make([]int, len(items))
	copy(out, items)
	return out
}
