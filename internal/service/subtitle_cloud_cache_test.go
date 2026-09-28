package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func testCloudSubtitleMedia(id, provider string) model.Media {
	return model.Media{
		Base: model.Base{ID: id},
		Path: "cloud://" + provider + "/Movies/Movie.mkv",
	}
}

func TestCloudSubtitleDiscoveryCacheCachesSuccessAndClonesTracks(t *testing.T) {
	var loads atomic.Int32
	cache := newCloudSubtitleDiscoveryCache(time.Hour)
	cache.load = func(context.Context, *SubtitleService, model.Media) ([]SubtitleTrack, error) {
		loads.Add(1)
		return []SubtitleTrack{{Path: "cloud://openlist/subtitle.srt", Lang: "zh-Hans"}}, nil
	}
	svc := &SubtitleService{cloudCache: cache}
	media := testCloudSubtitleMedia("media-1", "openlist")

	first, err := svc.discoverCloudSubtitlesCached(t.Context(), media)
	if err != nil {
		t.Fatal(err)
	}
	first[0].Lang = "changed"
	second, err := svc.discoverCloudSubtitlesCached(t.Context(), media)
	if err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 1 || second[0].Lang != "zh-Hans" {
		t.Fatalf("loads=%d tracks=%#v", loads.Load(), second)
	}
}

func TestCloudSubtitleDiscoveryCacheDoesNotCacheErrors(t *testing.T) {
	var loads atomic.Int32
	cache := newCloudSubtitleDiscoveryCache(time.Hour)
	cache.load = func(context.Context, *SubtitleService, model.Media) ([]SubtitleTrack, error) {
		if loads.Add(1) == 1 {
			return []SubtitleTrack{{Path: "cloud://openlist/partial.srt"}}, errors.New("temporary list failure")
		}
		return []SubtitleTrack{}, nil
	}
	svc := &SubtitleService{cloudCache: cache}
	media := testCloudSubtitleMedia("media-1", "openlist")

	partial, err := svc.discoverCloudSubtitlesCached(t.Context(), media)
	if err == nil {
		t.Fatal("first load error = nil")
	}
	if len(partial) != 1 {
		t.Fatalf("partial tracks=%#v, want preserved result", partial)
	}
	if _, err := svc.discoverCloudSubtitlesCached(t.Context(), media); err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 2 {
		t.Fatalf("loads=%d, want 2", loads.Load())
	}
}

func TestCloudSubtitleDiscoveryCacheCollapsesConcurrentLoads(t *testing.T) {
	var loads atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	cache := newCloudSubtitleDiscoveryCache(time.Hour)
	cache.load = func(context.Context, *SubtitleService, model.Media) ([]SubtitleTrack, error) {
		if loads.Add(1) == 1 {
			close(started)
		}
		<-release
		return []SubtitleTrack{}, nil
	}
	svc := &SubtitleService{cloudCache: cache}
	media := testCloudSubtitleMedia("media-1", "openlist")

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.discoverCloudSubtitlesCached(context.Background(), media)
			errCh <- err
		}()
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("loader did not start")
	}
	time.Sleep(10 * time.Millisecond)
	close(release)
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	if loads.Load() != 1 {
		t.Fatalf("loads=%d, want 1", loads.Load())
	}
}

func TestCloudSubtitleDiscoveryCacheInvalidatesByMediaAndProvider(t *testing.T) {
	var loads atomic.Int32
	cache := newCloudSubtitleDiscoveryCache(time.Hour)
	cache.load = func(context.Context, *SubtitleService, model.Media) ([]SubtitleTrack, error) {
		loads.Add(1)
		return []SubtitleTrack{}, nil
	}
	svc := &SubtitleService{cloudCache: cache}
	media1 := testCloudSubtitleMedia("media-1", "openlist")
	media2 := testCloudSubtitleMedia("media-2", "openlist")

	for _, media := range []model.Media{media1, media2} {
		if _, err := svc.discoverCloudSubtitlesCached(t.Context(), media); err != nil {
			t.Fatal(err)
		}
	}
	if invalidated := svc.InvalidateCloudDiscovery(media1.ID, ""); invalidated != 1 {
		t.Fatalf("media invalidated=%d, want 1", invalidated)
	}
	if _, err := svc.discoverCloudSubtitlesCached(t.Context(), media1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.discoverCloudSubtitlesCached(t.Context(), media2); err != nil {
		t.Fatal(err)
	}
	if loads.Load() != 3 {
		t.Fatalf("loads=%d, want 3", loads.Load())
	}
	if invalidated := svc.InvalidateCloudDiscovery("", "openlist"); invalidated != 2 {
		t.Fatalf("provider invalidated=%d, want 2", invalidated)
	}
}

func TestCloudSubtitleDiscoveryCacheDisabledLoadsEveryTime(t *testing.T) {
	var loads atomic.Int32
	cache := newCloudSubtitleDiscoveryCache(0)
	cache.load = func(context.Context, *SubtitleService, model.Media) ([]SubtitleTrack, error) {
		loads.Add(1)
		return []SubtitleTrack{}, nil
	}
	svc := &SubtitleService{cloudCache: cache}
	media := testCloudSubtitleMedia("media-1", "openlist")
	for range 2 {
		if _, err := svc.discoverCloudSubtitlesCached(t.Context(), media); err != nil {
			t.Fatal(err)
		}
	}
	if loads.Load() != 2 {
		t.Fatalf("loads=%d, want 2", loads.Load())
	}
}

func TestCloudSubtitleDiscoveryTTLFromEnv(t *testing.T) {
	t.Setenv("MEDIASTATION_SUBTITLE_CLOUD_CACHE_TTL_HOURS", "12")
	if got := cloudSubtitleDiscoveryTTLFromEnv(nil); got != 12*time.Hour {
		t.Fatalf("ttl=%v, want 12h", got)
	}
	t.Setenv("MEDIASTATION_SUBTITLE_CLOUD_CACHE_TTL_HOURS", "0")
	if got := cloudSubtitleDiscoveryTTLFromEnv(nil); got != 0 {
		t.Fatalf("ttl=%v, want disabled", got)
	}
	t.Setenv("MEDIASTATION_SUBTITLE_CLOUD_CACHE_TTL_HOURS", "999999999")
	if got := cloudSubtitleDiscoveryTTLFromEnv(nil); got != defaultCloudSubtitleDiscoveryTTL {
		t.Fatalf("ttl=%v, want default for out-of-range value", got)
	}
}
