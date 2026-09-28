package service

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"go.uber.org/zap"
)

func TestTranscoderStopIdleJobKeepsSecondViewer(t *testing.T) {
	cfg := &config.Config{}
	cfg.Transcoder.IdleTimeoutSeconds = 120
	cfg.Cache.CacheDir = t.TempDir()
	svc := NewTranscoderService(cfg, zap.NewNop(), nil, nil)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	job := &hlsJob{mediaID: "shared", cancel: cancel, lastAccess: time.Now().Add(-time.Hour)}
	svc.jobs[job.mediaID] = job
	if err := os.MkdirAll(svc.HLSDir(job.mediaID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc.HLSDir(job.mediaID), "seg_00000.ts"), []byte("segment"), 0o600); err != nil {
		t.Fatal(err)
	}
	stream := NewStreamService(cfg, zap.NewNop(), nil, svc)
	readSegment := func(viewer string) {
		t.Helper()
		req := httptest.NewRequest("GET", "/api/hls/shared/seg_00000.ts", nil)
		req.Header.Set("X-Test-Viewer", viewer)
		response := httptest.NewRecorder()
		if err := stream.ServeHLSSegment(response, req, job.mediaID, "seg_00000.ts"); err != nil {
			t.Fatal(err)
		}
		if response.Code != 200 || response.Body.String() != "segment" {
			t.Fatalf("viewer %s: %d %s", viewer, response.Code, response.Body.String())
		}
		if got := response.Header().Get("Cache-Control"); got != "private, no-store" {
			t.Fatalf("shared HLS URL must not enter a shared cache: %q", got)
		}
	}
	readSegment("first")
	readSegment("second")
	if svc.StopIdleJob(job.mediaID) {
		t.Fatal("first viewer's departure stopped an active shared job")
	}
	if ctx.Err() != nil || len(svc.Active()) != 1 {
		t.Fatal("second viewer's transcode was cancelled")
	}
	readSegment("second")
	if svc.StopIdleJob(job.mediaID) {
		t.Fatal("shared job stopped while second viewer was reading segments")
	}
	svc.mu.Lock()
	job.lastAccess = time.Now().Add(-3 * time.Minute)
	svc.mu.Unlock()
	if !svc.StopIdleJob(job.mediaID) || ctx.Err() == nil {
		t.Fatal("abandoned shared job was not reclaimed")
	}
}

func TestTranscoderOldWatchdogCannotStopReplacementJob(t *testing.T) {
	svc := NewTranscoderService(&config.Config{}, zap.NewNop(), nil, nil)
	old := &hlsJob{mediaID: "shared"}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	replacement := &hlsJob{mediaID: old.mediaID, cancel: cancel, lastAccess: time.Now().Add(-time.Hour)}
	svc.jobs[old.mediaID] = replacement
	if svc.stopIdleJob(old.mediaID, old, time.Now()) || ctx.Err() != nil {
		t.Fatal("old watchdog cancelled a replacement job")
	}
}
