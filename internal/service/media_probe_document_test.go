package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func probeDocumentFixture(t *testing.T) *ProbeResult {
	t.Helper()
	result, err := parseProbeJSON([]byte(`{
  "format":{"duration":"60", "filename":"https://private.example/file?token=secret", "tags":{"encoder":"private-encoder"}},
  "streams":[
    {"index":0,"codec_type":"video","codec_name":"mjpeg","width":200,"height":200,"disposition":{"attached_pic":1}},
    {"index":2,"codec_type":"video","codec_name":"hevc","width":3840,"height":2160,"avg_frame_rate":"24000/1001","color_transfer":"smpte2084"},
    {"index":5,"codec_type":"audio","codec_name":"aac","channels":2,"sample_rate":"48000","tags":{"language":"eng"},"disposition":{"default":1}},
    {"index":7,"codec_type":"audio","codec_name":"ac3","channels":6,"tags":{"language":"zho"}},
    {"index":9,"codec_type":"subtitle","codec_name":"ass","tags":{"language":"zho","title":"简体中文"},"disposition":{"forced":1}},
    {"index":11,"codec_type":"attachment","codec_name":"ttf"}
  ],"chapters":[{"id":0,"start_time":"0","end_time":"60","tags":{"title":"开场"}}]
}`))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProbeDocumentPreservesTracksAndRejectsPrivateFields(t *testing.T) {
	result := probeDocumentFixture(t)
	if result.VideoCodec != "hevc" || result.Width != 3840 || len(result.Document.Streams) != 4 {
		t.Fatalf("result=%+v", result)
	}
	raw, err := MarshalProbeDocument(result.Document)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private.example", "token", "private-encoder", "filename"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("unexpected private field %s", secret)
		}
	}
	doc, err := UnmarshalProbeDocument(raw, ProbeDocumentSchemaVersion)
	if err != nil || len(doc.Chapters) != 1 {
		t.Fatalf("document=%+v err=%v", doc, err)
	}
	streams := embyProbeStreams(doc)
	for i, index := range []int{2, 5, 7, 9} {
		if streams[i]["Index"] != index {
			t.Fatalf("stream index=%v", streams[i]["Index"])
		}
	}
	if streams[3]["Type"] != "Subtitle" || streams[3]["IsForced"] != true || nextMediaStreamIndex(streams) != 10 {
		t.Fatalf("streams=%+v", streams)
	}
	doc.Streams = append(doc.Streams, doc.Streams[0])
	if _, err := MarshalProbeDocument(doc); err == nil {
		t.Fatal("duplicate index accepted")
	}
	if _, err := UnmarshalProbeDocument(raw, ProbeDocumentSchemaVersion+1); err == nil {
		t.Fatal("future version accepted")
	}
}

func TestProbeDocumentPersistenceIsAtomicAndSourceBound(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.MediaProbeMetadata{})
	repos := repository.New(db)
	media := model.Media{Title: "source", Path: "/media/movie.mkv", SizeBytes: 500}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	probe := probeDocumentFixture(t)
	if err := persistMediaProbeResult(t.Context(), repos, nil, nil, nil, &media, probe); err != nil {
		t.Fatal(err)
	}
	if doc := loadMediaProbeDocument(t.Context(), repos, &media); doc == nil || len(doc.Streams) != 4 {
		t.Fatal("missing saved tracks")
	}
	changed := media
	changed.STRMURL = "https://example.com/changed"
	if doc := loadMediaProbeDocument(t.Context(), repos, &changed); doc != nil {
		t.Fatal("stale document returned")
	}
	if err := db.Model(&media).Update("path", "/media/replaced.mkv").Error; err != nil {
		t.Fatal(err)
	}
	media.Path = "/media/movie.mkv"
	if err := persistMediaProbeResult(t.Context(), repos, nil, nil, nil, &media, probe); !errors.Is(err, ErrMediaProbeSourceChanged) {
		t.Fatalf("expected source change: %v", err)
	}
	if err := db.First(&media, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	probe.DurationSec = 999
	probe.Document.Streams = append(probe.Document.Streams, probe.Document.Streams[0])
	if err := persistMediaProbeResult(t.Context(), repos, nil, nil, nil, &media, probe); err == nil {
		t.Fatal("invalid document saved")
	}
	var stored model.Media
	if err := db.First(&stored, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.DurationSec != 60 {
		t.Fatalf("failed document partially updated summary: %d", stored.DurationSec)
	}
	if err := db.Delete(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if err := persistMediaProbeResult(t.Context(), repos, nil, nil, nil, &media, probeDocumentFixture(t)); !errors.Is(err, ErrMediaProbeSourceChanged) {
		t.Fatalf("deleted media resurrected: %v", err)
	}
}

type recordingDocumentProber struct {
	paths  []string
	remote []string
	result *ProbeResult
	change func(string)
}

func (p *recordingDocumentProber) Probe(_ context.Context, path string) (*ProbeResult, error) {
	p.paths = append(p.paths, path)
	if p.change != nil {
		p.change(path)
	}
	return p.result, nil
}
func (p *recordingDocumentProber) ProbeHTTP(_ context.Context, u string, _ map[string]string) (*ProbeResult, error) {
	p.remote = append(p.remote, u)
	return p.result, nil
}

func TestProbeMappingsUseLongestPrefixPreservePlusAndRefuseTraversal(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nested, "电影+a.mkv")
	if err := os.WriteFile(file, []byte("video"), 0600); err != nil {
		t.Fatal(err)
	}
	mapping := "https://example.com/d => " + root + "\nhttps://example.com/d/movies => " + nested
	if got, _ := accessibleProbeMapping(mapping, "https://example.com/d/movies/%E7%94%B5%E5%BD%B1+a.mkv?sign=a%2Bb"); got != file {
		t.Fatalf("mapped=%q", got)
	}
	for _, u := range []string{"https://example.com/different/file.mkv", "https://example.com/d/%2e%2e/outside.mkv", "https://example.com/d/a%5c..%5coutside.mkv", "https://other.example/d/file.mkv"} {
		if got, _ := mapRemoteProbePathWithRoot(mapping, u); got != "" {
			t.Fatalf("unsafe mapping %s -> %s", u, got)
		}
	}
	db := newServiceTestDB(t, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), FFprobePathMappingsSettingKey, mapping); err != nil {
		t.Fatal(err)
	}
	s := NewStreamService(&config.Config{}, nil, repos, nil)
	p := &recordingDocumentProber{result: probeDocumentFixture(t)}
	m := &model.Media{Path: "/media/movie.strm", STRMURL: "https://example.com/d/movies/%E7%94%B5%E5%BD%B1+a.mkv"}
	if _, err := s.probeMediaSource(t.Context(), m, p); err != nil {
		t.Fatal(err)
	}
	if len(p.paths) != 1 || len(p.remote) != 0 {
		t.Fatalf("paths=%v remote=%v", p.paths, p.remote)
	}
	m.STRMURL = "https://example.com/d/movies/missing.mkv"
	if _, err := s.probeMediaSource(t.Context(), m, p); err != nil {
		t.Fatal(err)
	}
	if len(p.remote) != 1 {
		t.Fatal("unavailable local mapping must fall back to remote")
	}
	p.change = func(path string) {
		if err := os.WriteFile(path, []byte("changed video file"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := probeStableLocal(t.Context(), p, file); !errors.Is(err, ErrMediaProbeSourceChanged) {
		t.Fatalf("changed file accepted: %v", err)
	}
}

func TestProbeBackfillFindsScalarOnlyRowsWithinLibrary(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.MediaProbeMetadata{})
	repos := repository.New(db)
	rows := []model.Media{
		{Title: "old", LibraryID: "one", Path: "/media/one.mkv", MediaProbeVersion: 1, DurationSec: 60, Width: 1920, Height: 1080, VideoCodec: "h264", AudioCodec: "aac"},
		{Title: "other", LibraryID: "two", Path: "/media/two.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	s := NewStreamService(&config.Config{}, nil, repos, nil)
	p := &recordingDocumentProber{result: probeDocumentFixture(t)}
	result, err := s.ProbeMissingMediaInLibrary(t.Context(), p, 1, "one", nil)
	if err != nil || result.Total != 1 || result.Probed != 1 {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	result, err = s.ProbeMissingMediaInLibrary(t.Context(), p, 1, "one", nil)
	if err != nil || result.Total != 0 {
		t.Fatalf("repeat result=%+v err=%v", result, err)
	}
}
