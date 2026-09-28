package service

import (
	"context"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type streamProbeTestProber struct {
	localCalls int
	httpCalls  int
	headers    map[string]string
	result     *ProbeResult
}

func (p *streamProbeTestProber) Probe(_ context.Context, _ string) (*ProbeResult, error) {
	p.localCalls++
	return p.result, nil
}

func (p *streamProbeTestProber) ProbeHTTP(_ context.Context, _ string, headers map[string]string) (*ProbeResult, error) {
	p.httpCalls++
	p.headers = headers
	return p.result, nil
}

func TestStreamProbeCloudMediaUsesResolvedHTTPSource(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "其他媒体", Path: "cloud://openlist/115/其他", Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      "cloud://openlist/115/其他/movie.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2F115%2F%E5%85%B6%E4%BB%96%2Fmovie.mkv",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	resolver := &probeTestResolver{}
	prober := &streamProbeTestProber{result: &ProbeResult{
		DurationSec:   5400,
		Width:         1920,
		Height:        1080,
		VideoCodec:    "hevc",
		AudioCodec:    "aac",
		BitRate:       12000000,
		FrameRate:     23.976,
		VideoBitDepth: 10,
		AudioChannels: 2,
	}}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	svc.SetCloudProbe(resolver)
	if err := svc.Probe(t.Context(), media.ID, prober); err != nil {
		t.Fatal(err)
	}
	if resolver.ua != cloudMediaInternalUserAgent || prober.httpCalls != 1 || prober.localCalls != 0 || prober.headers["User-Agent"] != cloudMediaInternalUserAgent {
		t.Fatalf("resolver ua=%q local=%d http=%d headers=%#v", resolver.ua, prober.localCalls, prober.httpCalls, prober.headers)
	}
	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.DurationSec != 5400 || got.Width != 1920 || got.VideoCodec != "hevc" || got.BitRate != 12000000 || got.MediaProbeVersion != mediaProbeMetadataVersion {
		t.Fatalf("persisted media = %+v", got)
	}
}

func TestStreamProbeHTTPSTRMUsesRemoteProbe(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	media := model.Media{Title: "HTTP STRM", Path: "/media/http.strm", STRMURL: "https://cdn.example.test/video.mp4"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	prober := &streamProbeTestProber{result: &ProbeResult{DurationSec: 120, VideoCodec: "h264"}}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	if err := svc.Probe(t.Context(), media.ID, prober); err != nil {
		t.Fatal(err)
	}
	if prober.httpCalls != 1 || prober.localCalls != 0 || prober.headers["User-Agent"] != cloudMediaInternalUserAgent {
		t.Fatalf("local=%d http=%d headers=%#v", prober.localCalls, prober.httpCalls, prober.headers)
	}
}

func TestStreamProbeCloudMediaWithoutPlaybackReferenceReturnsExplicitError(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{Title: "Broken Cloud", Path: "cloud://openlist/115/其他/broken.mkv"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	prober := &streamProbeTestProber{result: &ProbeResult{DurationSec: 120}}
	svc := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	err := svc.Probe(t.Context(), media.ID, prober)
	if err == nil || !strings.Contains(err.Error(), "missing resolvable playback reference") {
		t.Fatalf("probe error = %v", err)
	}
	if prober.localCalls != 0 || prober.httpCalls != 0 {
		t.Fatalf("unexpected probe calls: local=%d http=%d", prober.localCalls, prober.httpCalls)
	}
}
