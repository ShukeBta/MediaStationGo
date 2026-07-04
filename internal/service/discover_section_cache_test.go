package service

import (
	"testing"
	"time"
)

func TestDiscoverSectionCacheReturnsClone(t *testing.T) {
	cache := NewDiscoverSectionCache(time.Hour)
	cache.Set("douban_hot_movie", 1, []ExternalMediaResult{{
		Title:            "第一部",
		SubscribeAliases: []string{"别名"},
		MissingEpisodes:  []int{1},
		Languages:        []string{"zh"},
	}})

	got, ok := cache.Get("douban_hot_movie", 1)
	if !ok || len(got) != 1 || got[0].Title != "第一部" {
		t.Fatalf("cached section = %#v, %v", got, ok)
	}

	got[0].Title = "被修改"
	got[0].SubscribeAliases[0] = "别名被改"
	got[0].MissingEpisodes[0] = 9
	got[0].Languages[0] = "en"
	again, ok := cache.Get("douban_hot_movie", 1)
	if !ok ||
		again[0].Title != "第一部" ||
		again[0].SubscribeAliases[0] != "别名" ||
		again[0].MissingEpisodes[0] != 1 ||
		again[0].Languages[0] != "zh" {
		t.Fatalf("cache should return a clone, got %#v", again)
	}
}

func TestDiscoverSectionCacheExpires(t *testing.T) {
	cache := NewDiscoverSectionCache(time.Nanosecond)
	cache.Set("tmdb_latest_movie", 1, []ExternalMediaResult{{Title: "旧数据"}})
	time.Sleep(time.Millisecond)

	if got, ok := cache.Get("tmdb_latest_movie", 1); ok || len(got) != 0 {
		t.Fatalf("expired cache should miss, got %#v", got)
	}
}
