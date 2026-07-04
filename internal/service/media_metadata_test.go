package service

import (
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestUpdateMediaMetadataMarksManualMatch(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "自采集", Path: "/media/custom", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "custom-media"}, LibraryID: lib.ID, Title: "raw", Path: "/media/custom/raw.mp4", ScrapeStatus: "no_match"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	title := "手动标题"
	overview := "手动简介"
	releaseDate := "2026-06-23"
	season := 0
	episode := 1
	tmdbID := 12345
	nsfw := true
	updated, err := svc.UpdateMetadata(t.Context(), media.ID, MediaMetadataUpdate{
		Title:       &title,
		Overview:    &overview,
		ReleaseDate: &releaseDate,
		SeasonNum:   &season,
		EpisodeNum:  &episode,
		TMDbID:      &tmdbID,
		NSFW:        &nsfw,
	})
	if err != nil {
		t.Fatalf("update metadata: %v", err)
	}
	if updated.Title != title || updated.Overview != overview || updated.ScrapeStatus != "matched" {
		t.Fatalf("metadata not saved: %#v", updated)
	}
	if updated.SeasonNum != 0 || updated.EpisodeNum != 1 || updated.TMDbID != tmdbID || !updated.NSFW {
		t.Fatalf("ids/episode metadata not saved: %#v", updated)
	}
	if updated.ReleaseDate != releaseDate {
		t.Fatalf("release date = %q, want %q", updated.ReleaseDate, releaseDate)
	}
}
