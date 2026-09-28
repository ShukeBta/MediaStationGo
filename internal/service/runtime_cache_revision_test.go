package service

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestRuntimeCacheMediaPrefixBumpsRevision(t *testing.T) {
	cache := NewRuntimeCacheService(&config.Config{}, zap.NewNop())
	if got := cache.Revision(context.Background(), "media"); got != 0 {
		t.Fatalf("initial media revision=%d, want 0", got)
	}
	cache.DeletePrefix(context.Background(), "media:")
	if got := cache.Revision(context.Background(), "media"); got != 1 {
		t.Fatalf("media revision after invalidation=%d, want 1", got)
	}
	cache.DeletePrefix(context.Background(), "media:browse:")
	if got := cache.Revision(context.Background(), "media"); got != 1 {
		t.Fatalf("partial prefix must not invalidate the media domain, got %d", got)
	}
}
