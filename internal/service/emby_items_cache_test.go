package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyReadCacheFlightWaiterReadsOwnerCache(t *testing.T) {
	svc := NewEmbyService(&config.Config{}, zap.NewNop(), nil).SetRuntimeCache(NewRuntimeCacheService(&config.Config{}, zap.NewNop()))
	key := "media:emby:test-flight"
	ownerCall, owner := svc.beginEmbyReadCacheFill(key)
	if !owner {
		t.Fatal("first cache fill should own the flight")
	}
	waiterCall, owner := svc.beginEmbyReadCacheFill(key)
	if owner {
		t.Fatal("second cache fill should wait for the owner")
	}
	if waiterCall != ownerCall {
		t.Fatal("waiter should observe the existing flight")
	}

	done := make(chan string, 1)
	go func() {
		if err := waitEmbyReadCacheFill(context.Background(), waiterCall); err != nil {
			done <- err.Error()
			return
		}
		var cached embyLatestCacheValue
		if !svc.cache.GetJSON(context.Background(), key, &cached) {
			done <- "cache miss after owner finished"
			return
		}
		cached.Items[0]["Name"] = "mutated"
		var again embyLatestCacheValue
		if !svc.cache.GetJSON(context.Background(), key, &again) {
			done <- "second cache miss"
			return
		}
		if again.Items[0]["Name"] != "Original" {
			done <- "cache returned shared mutable payload"
			return
		}
		done <- ""
	}()

	select {
	case msg := <-done:
		t.Fatalf("waiter returned before owner finished: %s", msg)
	case <-time.After(20 * time.Millisecond):
	}

	svc.cache.SetJSON(context.Background(), key, embyLatestCacheValue{Items: []map[string]any{{"Name": "Original"}}}, time.Minute)
	svc.finishEmbyReadCacheFill(key, ownerCall)

	select {
	case msg := <-done:
		if msg != "" {
			t.Fatal(msg)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter did not resume after owner finished")
	}
}

func TestInvalidateUserVisibilityChangesEmbyReadCacheKeys(t *testing.T) {
	svc := NewEmbyService(&config.Config{}, zap.NewNop(), nil)
	params := ItemsParams{UserID: "viewer", Limit: 20}
	itemsBefore := svc.embyItemsCacheKey("items", params)
	latestBefore := svc.embyLatestCacheKey("viewer", "", 20)
	svc.InvalidateUserVisibility("viewer")
	if itemsAfter := svc.embyItemsCacheKey("items", params); itemsAfter == itemsBefore {
		t.Fatal("items cache key must change immediately after a visibility update")
	}
	if latestAfter := svc.embyLatestCacheKey("viewer", "", 20); latestAfter == latestBefore {
		t.Fatal("latest cache key must change immediately after a visibility update")
	}
}

func TestEmbyItemsCacheKeySeparatesMediaSourcePayloads(t *testing.T) {
	svc := NewEmbyService(&config.Config{}, zap.NewNop(), nil)
	full := svc.embyItemsCacheKey("items", ItemsParams{Limit: 20})
	light := svc.embyItemsCacheKey("items", ItemsParams{Limit: 20, OmitMediaSources: true})
	if full == light {
		t.Fatal("full and lightweight item payloads must not share a cache key")
	}
}

func TestEmbyLatestItemsCacheReturnsIndependentPayload(t *testing.T) {
	svc := newTestEmbyService(t)
	svc.SetRuntimeCache(NewRuntimeCacheService(&config.Config{}, zap.NewNop()))
	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "movie-1", CreatedAt: time.Now()},
		LibraryID: lib.ID,
		Title:     "Original",
		Path:      `/media/movies/original.mkv`,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	first, err := svc.LatestItems(t.Context(), "user-1", lib.ID, 10)
	if err != nil {
		t.Fatalf("latest items: %v", err)
	}
	first[0]["Name"] = "mutated"
	second, err := svc.LatestItems(t.Context(), "user-1", lib.ID, 10)
	if err != nil {
		t.Fatalf("latest items from cache: %v", err)
	}
	if second[0]["Name"] != "Original" {
		t.Fatalf("cached latest payload was mutated: %#v", second[0])
	}
}

func TestEmbySeriesItemsCacheReturnsIndependentPayload(t *testing.T) {
	svc, lib := embyProjectionFixture(t, 3, 2)
	svc.SetRuntimeCache(NewRuntimeCacheService(&config.Config{}, zap.NewNop()))
	if _, err := svc.InitializeBrowseKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	params := ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Series"}, Recursive: true, Limit: 2}
	first, err := svc.Items(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	firstItems := first["Items"].([]map[string]any)
	firstItems[0]["Name"] = "mutated"
	second, err := svc.Items(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	secondItems := second["Items"].([]map[string]any)
	if secondItems[0]["Name"] == "mutated" {
		t.Fatalf("cached series payload was mutated: %#v", secondItems[0])
	}
}
