package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type batchProbeTestProber struct {
	started chan string
	release chan struct{}
	mu      sync.Mutex
	active  int
	maxSeen int
}

func (p *batchProbeTestProber) Probe(ctx context.Context, path string) (*ProbeResult, error) {
	p.mu.Lock()
	p.active++
	if p.active > p.maxSeen {
		p.maxSeen = p.active
	}
	p.mu.Unlock()
	p.started <- path
	select {
	case <-p.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	p.mu.Lock()
	p.active--
	p.mu.Unlock()
	if path == "/media/fail.mkv" {
		return nil, errors.New("probe failed")
	}
	return &ProbeResult{DurationSec: 120, Width: 1920, Height: 1080, VideoCodec: "h264", AudioCodec: "aac"}, nil
}

func (p *batchProbeTestProber) ProbeHTTP(ctx context.Context, path string, _ map[string]string) (*ProbeResult, error) {
	return p.Probe(ctx, path)
}

func TestProbeMissingMediaUsesTrackCompletenessAndConfiguredConcurrency(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	rows := []model.Media{
		{Title: "missing-ok", Path: "/media/ok.mkv", MediaProbeVersion: 1},
		{Title: "missing-fail", Path: "/media/fail.mkv", MediaProbeVersion: 1, DurationSec: 60},
		{Title: "complete", Path: "/media/complete.mkv", MediaProbeVersion: 1, DurationSec: 60, Width: 1280, Height: 720, VideoCodec: "h264", AudioCodec: "aac"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	prober := &batchProbeTestProber{started: make(chan string, 2), release: make(chan struct{})}
	stream := NewStreamService(&config.Config{}, zap.NewNop(), repository.New(db), nil)
	resultCh := make(chan MediaProbeBatchResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := stream.ProbeMissingMedia(t.Context(), prober, 2, nil)
		resultCh <- result
		errCh <- err
	}()

	started := map[string]bool{}
	for range 2 {
		select {
		case path := <-prober.started:
			started[path] = true
		case <-time.After(time.Second):
			t.Fatal("two probes did not start concurrently")
		}
	}
	if started["/media/complete.mkv"] {
		t.Fatal("complete media was probed")
	}
	close(prober.release)
	result := <-resultCh
	if err := <-errCh; err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Probed != 1 || result.Failed != 1 || len(result.Errors) != 1 {
		t.Fatalf("result = %+v", result)
	}
	prober.mu.Lock()
	maxSeen := prober.maxSeen
	prober.mu.Unlock()
	if maxSeen != 2 {
		t.Fatalf("max concurrent probes = %d, want 2", maxSeen)
	}
	var completed model.Media
	if err := db.First(&completed, "path = ?", "/media/ok.mkv").Error; err != nil {
		t.Fatal(err)
	}
	if completed.MediaProbeVersion != mediaProbeMetadataVersion || completed.VideoCodec != "h264" || completed.AudioCodec != "aac" {
		t.Fatalf("completed media = %+v", completed)
	}
}
