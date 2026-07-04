package service

import (
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestExistingCloudMediaSnapshotFiltersCloudRows(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)

	if err := db.Create(&[]model.Media{
		{
			LibraryID:    "lib-1",
			Path:         "cloud://openlist/Movie.mkv",
			SizeBytes:    2048,
			DurationSec:  120,
			Width:        1920,
			Height:       1080,
			VideoCodec:   "h264",
			AudioCodec:   "aac",
			Container:    "mkv",
			PosterURL:    "/poster.jpg",
			BackdropURL:  "/backdrop.jpg",
			STRMURL:      "/api/cloud/play/openlist?ref=movie",
			Year:         2026,
			TMDbID:       123,
			BangumiID:    456,
			DoubanID:     "douban-1",
			TheTVDBID:    "tvdb-1",
			ScrapeStatus: "matched",
		},
		{LibraryID: "lib-1", Path: "/media/local.mkv", SizeBytes: 99},
		{LibraryID: "lib-2", Path: "cloud://openlist/Other.mkv", SizeBytes: 88},
	}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := scanner.existingCloudMediaSnapshot(t.Context(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("snapshot len = %d, want 1: %#v", len(got), got)
	}
	row := got["cloud://openlist/Movie.mkv"]
	if row.SizeBytes != 2048 || row.DurationSec != 120 || row.Width != 1920 || row.Height != 1080 {
		t.Fatalf("track fields not preserved: %#v", row)
	}
	if row.VideoCodec != "h264" || row.AudioCodec != "aac" || row.Container != "mkv" {
		t.Fatalf("codec fields not preserved: %#v", row)
	}
	if row.PosterURL != "/poster.jpg" || row.BackdropURL != "/backdrop.jpg" || row.STRMURL == "" {
		t.Fatalf("artwork/strm fields not preserved: %#v", row)
	}
	if row.Year != 2026 || row.TMDbID != 123 || row.BangumiID != 456 || row.DoubanID != "douban-1" || row.TheTVDBID != "tvdb-1" {
		t.Fatalf("scraper ids not preserved: %#v", row)
	}
}

func TestExistingLocalMediaSnapshotFiltersAndCleansLocalRows(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)

	rawPath := filepath.Join("D:", "media", "Movies", "..", "Movie.mkv")
	cleanPath := filepath.Clean(rawPath)
	if err := db.Create(&[]model.Media{
		{
			LibraryID:    "lib-1",
			Path:         rawPath,
			SizeBytes:    4096,
			DurationSec:  240,
			Width:        3840,
			Height:       2160,
			VideoCodec:   "hevc",
			AudioCodec:   "truehd",
			Container:    "mkv",
			STRMURL:      "https://cdn.example.com/movie.mkv",
			FileID:       "dev:inode",
			ScrapeStatus: "matched",
		},
		{LibraryID: "lib-1", Path: "cloud://openlist/Movie.mkv", SizeBytes: 99},
		{LibraryID: "lib-2", Path: filepath.Join("D:", "media", "Other.mkv"), SizeBytes: 88},
	}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := scanner.existingLocalMediaSnapshot(t.Context(), "lib-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("snapshot len = %d, want 1: %#v", len(got), got)
	}
	row, ok := got[cleanPath]
	if !ok {
		t.Fatalf("snapshot key %q not found in %#v", cleanPath, got)
	}
	if row.SizeBytes != 4096 || row.DurationSec != 240 || row.Width != 3840 || row.Height != 2160 {
		t.Fatalf("track fields not preserved: %#v", row)
	}
	if row.VideoCodec != "hevc" || row.AudioCodec != "truehd" || row.Container != "mkv" {
		t.Fatalf("codec fields not preserved: %#v", row)
	}
	if row.STRMURL == "" || row.FileID != "dev:inode" {
		t.Fatalf("identity fields not preserved: %#v", row)
	}
}
