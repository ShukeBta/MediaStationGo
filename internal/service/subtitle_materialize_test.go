package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
	"go.uber.org/zap"
)

type cloudSubtitleOpenListFixture struct {
	server       *httptest.Server
	listCalls    atomic.Int32
	readCalls    atomic.Int32
	refreshMu    sync.Mutex
	refreshFlags []bool
}

func newCloudSubtitleOpenListFixture(t *testing.T, failRead bool) *cloudSubtitleOpenListFixture {
	t.Helper()
	fixture := &cloudSubtitleOpenListFixture{}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			fixture.listCalls.Add(1)
			var req struct {
				Path    string `json:"path"`
				Refresh bool   `json:"refresh"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			fixture.refreshMu.Lock()
			fixture.refreshFlags = append(fixture.refreshFlags, req.Refresh)
			fixture.refreshMu.Unlock()
			var entries []map[string]any
			switch req.Path {
			case "/115/movie/Movie":
				entries = []map[string]any{
					{"name": "Movie.mkv", "size": 5000, "is_dir": false},
					{"name": "Movie.zh.srt", "size": 128, "is_dir": false},
					{"name": "Subtitles", "size": 0, "is_dir": true},
				}
			case "/115/movie/Movie/Subtitles":
				entries = []map[string]any{{"name": "commentary.ass", "size": 64, "is_dir": false}}
			default:
				http.Error(w, "unexpected list path "+req.Path, http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"content": entries, "total": len(entries)}})
		case "/dav/115/movie/Movie/Movie.zh.srt":
			fixture.readCalls.Add(1)
			if failRead {
				http.Error(w, "subtitle unavailable", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhello\n"))
		case "/dav/115/movie/Movie/Subtitles/commentary.ass":
			fixture.readCalls.Add(1)
			if failRead {
				http.Error(w, "subtitle unavailable", http.StatusBadGateway)
				return
			}
			_, _ = w.Write([]byte("[Script Info]\nTitle: commentary\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *cloudSubtitleOpenListFixture) refreshes() []bool {
	f.refreshMu.Lock()
	defer f.refreshMu.Unlock()
	return append([]bool(nil), f.refreshFlags...)
}

func TestCloudSubtitleMaterializePersistsTracksAndPlaybackStaysLocal(t *testing.T) {
	fixture := newCloudSubtitleOpenListFixture(t, false)
	db := newServiceTestDB(t, &model.Media{}, &model.StorageConfig{})
	repos := repository.New(db)
	media := model.Media{Base: model.Base{ID: "movie-materialized"}, Title: "Movie", Path: "cloud://openlist/115/movie/Movie/Movie.mkv"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": fixture.server.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}

	localCacheDir := t.TempDir()
	localMediaDir := filepath.Join(localCacheDir, media.ID)
	if err := os.MkdirAll(localMediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localMediaDir, "manual.srt"), []byte("manual"), 0o644); err != nil {
		t.Fatal(err)
	}
	localIndex, err := json.Marshal(localSubtitleIndex{Tracks: []localSubtitleIndexTrack{{MediaID: media.ID, Filename: "manual.srt", Source: "subtitlecat"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localMediaDir, "tracks.json"), localIndex, 0o644); err != nil {
		t.Fatal(err)
	}

	cacheDir := t.TempDir()
	svc := NewSubtitleService(log, repos, storage)
	svc.SetLocalCacheDir(localCacheDir)
	svc.SetMaterializedCacheDir(cacheDir)
	result, err := svc.EnsureCloudSubtitles(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || result.Discovered != 2 || result.Cached != 2 {
		t.Fatalf("materialize result=%#v", result)
	}
	if fixture.listCalls.Load() != 2 || fixture.readCalls.Load() != 2 {
		t.Fatalf("list=%d read=%d, want 2/2", fixture.listCalls.Load(), fixture.readCalls.Load())
	}
	again, err := svc.EnsureCloudSubtitles(t.Context(), media.ID)
	if err != nil || again.Status != "skipped" || again.Reason != "cache_current" {
		t.Fatalf("second materialize=%#v err=%v", again, err)
	}
	refreshed, err := svc.RefreshCloudSubtitles(t.Context(), media.ID)
	if err != nil || refreshed.Cached != 2 {
		t.Fatalf("refresh result=%#v err=%v", refreshed, err)
	}
	if got := fixture.refreshes(); len(got) != 4 || got[0] || got[1] || !got[2] || !got[3] {
		t.Fatalf("refresh flags=%v, want false false true true", got)
	}

	svc.storage = nil
	tracks, err := svc.Discover(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 3 || fixture.listCalls.Load() != 4 || fixture.readCalls.Load() != 4 {
		t.Fatalf("tracks=%#v list=%d read=%d", tracks, fixture.listCalls.Load(), fixture.readCalls.Load())
	}
	cloudTracks := 0
	var srtTrack SubtitleTrack
	for _, track := range tracks {
		if track.Source == materializedSubtitleSource {
			cloudTracks++
			if track.Codec == "srt" {
				srtTrack = track
			}
		}
	}
	if cloudTracks != 2 || srtTrack.Path == "" {
		t.Fatalf("materialized tracks=%#v", tracks)
	}
	var body bytes.Buffer
	if err := svc.ServeAs(t.Context(), media.ID, srtTrack.Path, ".srt", &body); err != nil {
		t.Fatal(err)
	}
	if body.String() != "1\n00:00:01,000 --> 00:00:02,000\nhello" {
		t.Fatalf("cached subtitle body=%q", body.String())
	}
}

func TestCloudSubtitleEmptyMaterializationExpiresAndScansAgain(t *testing.T) {
	var listCalls atomic.Int32
	var exposeSubtitle atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/fs/list":
			listCalls.Add(1)
			entries := []map[string]any{{"name": "Movie.mkv", "size": 5000, "is_dir": false}}
			if exposeSubtitle.Load() {
				entries = append(entries, map[string]any{"name": "Movie.zh.srt", "size": 64, "is_dir": false})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "data": map[string]any{"content": entries, "total": len(entries)}})
		case "/dav/115/movie/Movie/Movie.zh.srt":
			_, _ = w.Write([]byte("subtitle"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	db := newServiceTestDB(t, &model.Media{}, &model.StorageConfig{})
	repos := repository.New(db)
	media := model.Media{Base: model.Base{ID: "movie-empty-materialized"}, Path: "cloud://openlist/115/movie/Movie/Movie.mkv"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{Type: "openlist", Config: map[string]any{"server": server.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	cacheDir := t.TempDir()
	svc := NewSubtitleService(log, repos, storage)
	svc.SetMaterializedCacheDir(cacheDir)
	first, err := svc.EnsureCloudSubtitles(t.Context(), media.ID)
	if err != nil || first.Cached != 0 || listCalls.Load() != 1 {
		t.Fatalf("first=%#v err=%v list=%d", first, err, listCalls.Load())
	}
	exposeSubtitle.Store(true)
	second, err := svc.EnsureCloudSubtitles(t.Context(), media.ID)
	if err != nil || second.Reason != "cache_current" || listCalls.Load() != 1 {
		t.Fatalf("second=%#v err=%v list=%d", second, err, listCalls.Load())
	}
	indexPath := filepath.Join(cacheDir, media.ID, "tracks.json")
	index, exists, err := readMaterializedSubtitleIndex(cacheDir, media.ID)
	if err != nil || !exists {
		t.Fatalf("read index exists=%v err=%v", exists, err)
	}
	index.CachedAt = time.Now().Add(-materializedCloudSubtitleFreshness - time.Minute).UTC().Format(time.RFC3339Nano)
	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(indexPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	third, err := svc.EnsureCloudSubtitles(t.Context(), media.ID)
	if err != nil || third.Cached != 1 || listCalls.Load() != 2 {
		t.Fatalf("third=%#v err=%v list=%d", third, err, listCalls.Load())
	}
}

func TestCloudSubtitleTracksDoNotCrossEpisodeBasenames(t *testing.T) {
	entries := []cloud.FileEntry{
		{ID: "/shows/Show.S01E01.zh.srt", Name: "Show.S01E01.zh.srt"},
		{ID: "/shows/Show.S01E02.zh.srt", Name: "Show.S01E02.zh.srt"},
		{ID: "/shows/Show.S01E010.zh.srt", Name: "Show.S01E010.zh.srt"},
	}
	tracks := cloudSubtitleTracks("openlist", entries, "Show.S01E01", true)
	if len(tracks) != 1 || !strings.Contains(tracks[0].Path, "Show.S01E01.zh.srt") {
		t.Fatalf("tracks=%#v, want only S01E01", tracks)
	}
}

func TestBackfillCloudSubtitlesReturnsFailureSummary(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{Base: model.Base{ID: "backfill-failure"}, Path: "cloud://openlist/115/movie/Movie/Movie.mkv"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewSubtitleService(zap.NewNop(), repos)
	svc.SetMaterializedCacheDir(t.TempDir())
	result, err := svc.BackfillCloudSubtitles(t.Context(), "", nil)
	if err == nil || result.Total != 1 || result.Processed != 1 || result.Failed != 1 || len(result.Errors) != 1 {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestDeleteMaterializedCloudSubtitleRequiresRefresh(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{Base: model.Base{ID: "materialized-delete"}, Path: "cloud://openlist/115/movie/Movie/Movie.mkv"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	cacheDir := t.TempDir()
	mediaDir := filepath.Join(cacheDir, media.ID)
	if err := os.MkdirAll(mediaDir, 0o755); err != nil {
		t.Fatal(err)
	}
	filename := "cloud-deadbeef.srt"
	if err := os.WriteFile(filepath.Join(mediaDir, filename), []byte("subtitle"), 0o644); err != nil {
		t.Fatal(err)
	}
	index := materializedSubtitleIndex{
		MediaID: media.ID, Provider: "openlist", MediaPath: media.Path, CachedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Tracks: []materializedSubtitleIndexTrack{{Filename: filename, Name: "Movie.srt"}},
	}
	raw, err := json.Marshal(index)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaDir, "tracks.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	svc := NewSubtitleService(zap.NewNop(), repos)
	svc.SetMaterializedCacheDir(cacheDir)
	err = svc.Delete(t.Context(), media.ID, materializedSubtitleURI(media.ID, filename))
	if err == nil || !strings.Contains(err.Error(), "managed by cloud subtitle refresh") {
		t.Fatalf("delete error=%v", err)
	}
	if _, statErr := os.Stat(filepath.Join(mediaDir, filename)); statErr != nil {
		t.Fatalf("materialized subtitle was removed: %v", statErr)
	}
}

func TestFailedCloudSubtitleMaterializeResultKeepsOriginalError(t *testing.T) {
	want := errors.New("download failed")
	result, err := failedCloudSubtitleMaterializeResult(CloudSubtitleMaterializeResult{MediaID: "m1"}, want)
	if !errors.Is(err, want) || result.Status != "failed" || result.Error != want.Error() {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
