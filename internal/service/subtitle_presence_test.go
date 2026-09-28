package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type subtitlePresenceProbe struct {
	result *ProbeResult
	calls  int
}

func (p *subtitlePresenceProbe) Probe(context.Context, string) (*ProbeResult, error) {
	p.calls++
	return p.result, nil
}

func (p *subtitlePresenceProbe) ProbeHTTP(context.Context, string, map[string]string) (*ProbeResult, error) {
	p.calls++
	return p.result, nil
}

func TestSubtitlePresenceDetectsUnlabelledChineseExternalSubtitleBeforeProbe(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	dir := t.TempDir()
	media := model.Media{Base: model.Base{ID: "media-external-zh"}, Title: "Movie", Path: filepath.Join(dir, "Movie.mkv")}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Movie.srt"), []byte("1\n00:00:01,000 --> 00:00:02,000\n这是一段中文字幕内容\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	subtitles := NewSubtitleService(zap.NewNop(), repos)
	stream := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	probe := &subtitlePresenceProbe{result: &ProbeResult{}}

	result, err := subtitles.Presence(t.Context(), media.ID, stream, probe)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasChinese || result.EmbeddedChecked || len(result.External) != 1 || !result.External[0].Chinese || probe.calls != 0 {
		t.Fatalf("presence = %+v probe_calls=%d", result, probe.calls)
	}
}

func TestSubtitlePresenceDetectsChineseEmbeddedSubtitle(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	dir := t.TempDir()
	media := model.Media{Base: model.Base{ID: "media-embedded-zh"}, Title: "Movie", Path: filepath.Join(dir, "Movie.mkv")}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	subtitles := NewSubtitleService(zap.NewNop(), repos)
	stream := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	probe := &subtitlePresenceProbe{result: &ProbeResult{SubtitleStreams: []ProbeSubtitleStream{{Index: 3, Codec: "ass", Language: "zho", Title: "简体中文"}}}}

	result, err := subtitles.Presence(t.Context(), media.ID, stream, probe)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasChinese || !result.EmbeddedChecked || len(result.Embedded) != 1 || !result.Embedded[0].Chinese || probe.calls != 1 {
		t.Fatalf("presence = %+v probe_calls=%d", result, probe.calls)
	}
}

func TestSubtitlePresenceReportsNoChineseWhenTracksAreEnglish(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	dir := t.TempDir()
	media := model.Media{Base: model.Base{ID: "media-english"}, Title: "Movie", Path: filepath.Join(dir, "Movie.mkv")}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Movie.en.srt"), []byte("English subtitle"), 0o644); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	subtitles := NewSubtitleService(zap.NewNop(), repos)
	stream := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	probe := &subtitlePresenceProbe{result: &ProbeResult{SubtitleStreams: []ProbeSubtitleStream{{Index: 2, Codec: "subrip", Language: "eng"}}}}

	result, err := subtitles.Presence(t.Context(), media.ID, stream, probe)
	if err != nil {
		t.Fatal(err)
	}
	if result.HasChinese || !result.EmbeddedChecked || len(result.External) != 1 || len(result.Embedded) != 1 {
		t.Fatalf("presence = %+v", result)
	}
}

func TestSubtitlePresenceDoesNotTreatDiscoveryFailureAsNoSubtitle(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	media := model.Media{Base: model.Base{ID: "media-missing-dir"}, Title: "Movie", Path: filepath.Join(t.TempDir(), "missing", "Movie.mkv")}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	subtitles := NewSubtitleService(zap.NewNop(), repos)
	stream := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	probe := &subtitlePresenceProbe{result: &ProbeResult{}}

	if _, err := subtitles.Presence(t.Context(), media.ID, stream, probe); err == nil {
		t.Fatal("expected subtitle discovery failure")
	}
	if probe.calls != 0 {
		t.Fatalf("probe calls = %d, want 0", probe.calls)
	}
}

func TestSubtitlePresenceDoesNotTreatBrokenCacheIndexAsNoSubtitle(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	dir := t.TempDir()
	media := model.Media{Base: model.Base{ID: "media-broken-cache"}, Title: "Movie", Path: filepath.Join(dir, "Movie.mkv")}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	cacheRoot := t.TempDir()
	cacheDir := filepath.Join(cacheRoot, media.ID)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "tracks.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	subtitles := NewSubtitleService(zap.NewNop(), repos)
	subtitles.SetLocalCacheDir(cacheRoot)
	stream := NewStreamService(&config.Config{}, zap.NewNop(), repos, nil)
	probe := &subtitlePresenceProbe{result: &ProbeResult{}}

	if _, err := subtitles.Presence(t.Context(), media.ID, stream, probe); err == nil {
		t.Fatal("expected cached subtitle index failure")
	}
	if probe.calls != 0 {
		t.Fatalf("probe calls = %d, want 0", probe.calls)
	}
}
