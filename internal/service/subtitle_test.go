package service

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubtitleDiscoverNoTracksReturnsEmptySlice(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	media := model.Media{
		Title: "No Subtitles",
		Path:  filepath.Join(dir, "No Subtitles.mkv"),
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewSubtitleService(zap.NewNop(), repository.New(db))
	tracks, err := svc.Discover(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tracks == nil {
		t.Fatal("tracks is nil, want empty slice")
	}
	if len(tracks) != 0 {
		t.Fatalf("len(tracks) = %d, want 0", len(tracks))
	}
}

func TestSubtitleDiscoverMergesLocalCacheTracks(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	media := model.Media{
		Base:  model.Base{ID: "media-cached"},
		Title: "Cached Subtitle",
		Path:  filepath.Join(dir, "Cached Subtitle.mkv"),
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	cacheDir := t.TempDir()
	mediaCacheDir := filepath.Join(cacheDir, media.ID)
	if err := os.MkdirAll(mediaCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte("1\n00:00:01,000 --> 00:00:02,000\nhello\n")
	if err := os.WriteFile(filepath.Join(mediaCacheDir, "subtitlecat-test.srt"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaCacheDir, "tracks.json"), []byte(`{
  "media_id": "media-cached",
  "tracks": [
    {
      "media_id": "media-cached",
      "filename": "subtitlecat-test.srt",
      "name": "星际迷航.新世界.S01E01.HDR.2160p.WEBRip.HEVC.chs.eng-STCFanSub",
      "lang": "zh-Hans",
      "label": "简体中文",
      "source": "subhd",
      "provider_id": "subtitle-123"
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewSubtitleService(zap.NewNop(), repository.New(db))
	svc.SetLocalCacheDir(cacheDir)
	tracks, err := svc.Discover(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 {
		t.Fatalf("len(tracks) = %d, want 1: %#v", len(tracks), tracks)
	}
	if tracks[0].Path != "local-subtitle://media-cached/subtitlecat-test.srt" || tracks[0].Name != "星际迷航.新世界.S01E01.HDR.2160p.WEBRip.HEVC.chs.eng-STCFanSub" || tracks[0].Lang != "zh-Hans" || tracks[0].Label != "简体中文" || tracks[0].Codec != "srt" || tracks[0].Source != "subhd" || tracks[0].ApplicationKey != subtitleApplicationKey("subhd", "subtitle-123") {
		t.Fatalf("unexpected local cache track: %#v", tracks[0])
	}

	var native bytes.Buffer
	if err := svc.ServeAs(t.Context(), media.ID, tracks[0].Path, ".srt", &native); err != nil {
		t.Fatalf("serve srt: %v", err)
	}
	if native.String() != string(body) {
		t.Fatalf("native subtitle body = %q, want %q", native.String(), string(body))
	}

	var vtt bytes.Buffer
	if err := svc.ServeAs(t.Context(), media.ID, tracks[0].Path, ".vtt", &vtt); err != nil {
		t.Fatalf("serve vtt: %v", err)
	}
	if !strings.HasPrefix(vtt.String(), "WEBVTT") || !strings.Contains(vtt.String(), "hello") {
		t.Fatalf("unexpected vtt body: %q", vtt.String())
	}
}

func TestSubtitleServeAsKeepsSUPBinaryBody(t *testing.T) {
	body := []byte{0x50, 0x47, 0x53, 0x00, 0x01}
	var output bytes.Buffer
	if err := writeSubtitleBody(&output, body, ".sup", ".sup"); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), body) {
		t.Fatalf("SUP body = %x, want %x", output.Bytes(), body)
	}
	if contentType, ok := SubtitleContentType(".sup"); !ok || contentType != "application/octet-stream" {
		t.Fatalf("SUP content type = %q, %v", contentType, ok)
	}
}

func TestSubtitleDiscoverCloudMediaUsesOnlyLocalCache(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:subtitle-cloud-local-only?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}

	media := model.Media{
		Base:  model.Base{ID: "media-cloud-cached"},
		Title: "Cloud Cached Subtitle",
		Path:  "cloud://openlist/115/Movies/Cloud Cached Subtitle.mkv",
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	cacheDir := t.TempDir()
	mediaCacheDir := filepath.Join(cacheDir, media.ID)
	if err := os.MkdirAll(mediaCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaCacheDir, "cached.srt"), []byte("cached subtitle"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mediaCacheDir, "tracks.json"), []byte(`{
  "media_id": "media-cloud-cached",
  "tracks": [
    {
      "media_id": "media-cloud-cached",
      "filename": "cached.srt",
      "lang": "zh-Hans",
      "label": "简体中文",
      "source": "cache"
    }
  ]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cloudLoads := 0
	svc := NewSubtitleService(zap.NewNop(), repository.New(db))
	svc.SetLocalCacheDir(cacheDir)
	svc.cloudCache.load = func(context.Context, *SubtitleService, model.Media) ([]SubtitleTrack, error) {
		cloudLoads++
		return []SubtitleTrack{{Path: "cloud://openlist/should-not-be-returned.srt"}}, nil
	}

	tracks, err := svc.Discover(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if cloudLoads != 0 {
		t.Fatalf("cloud subtitle loads = %d, want 0", cloudLoads)
	}
	if len(tracks) != 1 {
		t.Fatalf("len(tracks) = %d, want 1: %#v", len(tracks), tracks)
	}
	if tracks[0].Path != "local-subtitle://media-cloud-cached/cached.srt" || tracks[0].Source != "cache" {
		t.Fatalf("unexpected cached cloud-media track: %#v", tracks[0])
	}
}

func TestSubtitleDeleteRemovesOnlySelectedCachedTrackAndUpdatesIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:subtitle-delete?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:  model.Base{ID: "media-delete-subtitle"},
		Title: "Delete Subtitle",
		Path:  filepath.Join(t.TempDir(), "Delete Subtitle.mkv"),
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	cacheDir := t.TempDir()
	mediaCacheDir := filepath.Join(cacheDir, media.ID)
	if err := os.MkdirAll(mediaCacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"subtitlecat-first.srt", "assrt-second.ass"} {
		if err := os.WriteFile(filepath.Join(mediaCacheDir, name), []byte("subtitle"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index := `{"media_id":"media-delete-subtitle","tracks":[` +
		`{"media_id":"media-delete-subtitle","filename":"subtitlecat-first.srt","lang":"zh-Hans","label":"简体中文","source":"subtitlecat","provider_id":"first"},` +
		`{"media_id":"media-delete-subtitle","filename":"assrt-second.ass","lang":"zh-Hans","label":"简体中文","source":"assrt","provider_id":"second"}` +
		`]}`
	if err := os.WriteFile(filepath.Join(mediaCacheDir, "tracks.json"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}

	svc := NewSubtitleService(zap.NewNop(), repository.New(db))
	svc.SetLocalCacheDir(cacheDir)
	if err := svc.Delete(t.Context(), media.ID, localSubtitleURI(media.ID, "subtitlecat-first.srt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(mediaCacheDir, "subtitlecat-first.srt")); !os.IsNotExist(err) {
		t.Fatalf("deleted subtitle still exists: %v", err)
	}
	tracks, err := svc.Discover(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].Name != "assrt-second.ass" || tracks[0].Source != "assrt" {
		t.Fatalf("unexpected remaining tracks: %#v", tracks)
	}
	if err := svc.Delete(t.Context(), media.ID, localSubtitleURI(media.ID, "not-indexed.srt")); err == nil {
		t.Fatal("expected unowned subtitle deletion to fail")
	}
}
