package service

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestNormaliseTimecode(t *testing.T) {
	cases := map[string]string{
		"0:00:01":      "00:00:01",
		"0:00:01.51":   "00:00:01.510", // ASS centiseconds -> 3-digit ms
		"0:00:04.123":  "00:00:04.123", // already 3 digits
		"00:00:12.07":  "00:00:12.070", // 2-digit fraction padded
		"00:01:30.1234": "00:01:30.123", // capped at 3 digits
		"00:00:1.5":    "00:00:01.500", // single-digit seconds and fraction
	}
	for in, want := range cases {
		if got := normaliseTimecode(in); got != want {
			t.Errorf("normaliseTimecode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSubtitleServeRawWritesSourceBytes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	videoPath := filepath.Join(dir, "MovieName.mkv")
	subPath := filepath.Join(dir, "MovieName.ass")
	raw := "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,線合なライン\n"
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(subPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	media := model.Media{Title: "MovieName", Path: videoPath}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewSubtitleService(zap.NewNop(), repository.New(db))
	var buf bytes.Buffer
	if err := svc.ServeRaw(t.Context(), media.ID, subPath, &buf); err != nil {
		t.Fatal(err)
	}
	// ServeRaw must NOT convert ASS->VTT; it returns the exact source bytes.
	if got := buf.String(); got != raw {
		t.Fatalf("ServeRaw returned %q, want raw %q", got, raw)
	}
}
