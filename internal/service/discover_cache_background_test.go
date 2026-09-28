package service

import (
	"testing"
	"time"
)

func TestDiscoverSectionCacheKeepsLastGoodAfterTTL(t *testing.T) {
	cache := NewDiscoverSectionCache(time.Nanosecond)
	cache.Set("adult_javdb_popular", 1, []ExternalMediaResult{{Title: "last good"}})
	time.Sleep(time.Millisecond)

	if got, ok := cache.Get("adult_javdb_popular", 1); ok || len(got) != 0 {
		t.Fatalf("expired normal cache should miss, got %#v", got)
	}
	got, ok := cache.GetLastGood("adult_javdb_popular", 1)
	if !ok || len(got) != 1 || got[0].Title != "last good" {
		t.Fatalf("last good cache = %#v, %v", got, ok)
	}
}
