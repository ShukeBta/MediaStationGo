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
		PreviewImages:    []string{"https://img.example/preview.jpg"},
		Languages:        []string{"zh"},
	}})

	got, ok := cache.Get("douban_hot_movie", 1)
	if !ok || len(got) != 1 || got[0].Title != "第一部" {
		t.Fatalf("cached section = %#v, %v", got, ok)
	}

	got[0].Title = "被修改"
	got[0].SubscribeAliases[0] = "别名被改"
	got[0].MissingEpisodes[0] = 9
	got[0].PreviewImages[0] = "https://img.example/changed.jpg"
	got[0].Languages[0] = "en"
	again, ok := cache.Get("douban_hot_movie", 1)
	if !ok ||
		again[0].Title != "第一部" ||
		again[0].SubscribeAliases[0] != "别名" ||
		again[0].MissingEpisodes[0] != 1 ||
		again[0].PreviewImages[0] != "https://img.example/preview.jpg" ||
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

func TestDiscoverSectionCacheDeleteSection(t *testing.T) {
	cache := NewDiscoverSectionCache(time.Hour)
	cache.Set("adult_followed:user-1", 1, []ExternalMediaResult{{Title: "A"}})
	cache.Set("adult_followed:user-1", 2, []ExternalMediaResult{{Title: "B"}})
	cache.Set("adult_followed:user-2", 1, []ExternalMediaResult{{Title: "C"}})
	cache.DeleteSection("adult_followed:user-1")
	if _, ok := cache.Get("adult_followed:user-1", 1); ok {
		t.Fatal("page 1 should be invalidated")
	}
	if _, ok := cache.Get("adult_followed:user-1", 2); ok {
		t.Fatal("page 2 should be invalidated")
	}
	if _, ok := cache.Get("adult_followed:user-2", 1); !ok {
		t.Fatal("another user's cache must remain")
	}
}
