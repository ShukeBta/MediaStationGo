package service

import (
	"fmt"
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
	if c == nil || key == "" || page < 1 {
		return nil, false
	}
	c.mu.RLock()
	entry, ok := c.entries[discoverSectionCacheKey(key, page)]
	c.mu.RUnlock()
	if !ok || time.Since(entry.storedAt) > c.ttl || len(entry.items) == 0 {
		return nil, false
	}
	return cloneExternalMediaResults(entry.items), true
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
		out[i].Languages = cloneStrings(item.Languages)
		out[i].Countries = cloneStrings(item.Countries)
		out[i].Genres = item.Genres
	}
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
