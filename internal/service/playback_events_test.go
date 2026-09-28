package service

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestPlaybackEventsDeduplicateAndPreserveHistoryIndependence(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{}, &model.PlaybackEvent{}, &model.User{}, &model.Library{})
	repos := repository.New(db)
	media := model.Media{Base: model.Base{ID: "media"}, LibraryID: "library", Title: "Example", Path: "/example.mp4", TMDbID: 7, DurationSec: 1200}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	c := &Container{Repo: repos}
	for _, position := range []int64{10_000, 30_000, 60_000, 90_000} {
		if err := c.RecordPlaybackEvent(t.Context(), "viewer", media.ID, "session-1", "Web", position, 1200000); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.RecordPlaybackEvent(t.Context(), "viewer", media.ID, "session-2", "Web", 60_000, 1200000); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&model.PlaybackEvent{}).Count(&count).Error; err != nil || count != 2 {
		t.Fatalf("events = %d, %v", count, err)
	}
	if err := db.Delete(&media).Error; err != nil {
		t.Fatal(err)
	}
	from := time.Now().UTC().Truncate(24 * time.Hour)
	stats, err := repos.PlaybackStats(t.Context(), repository.PlaybackStatsFilter{From: from, To: from.Add(24 * time.Hour), RankFrom: from, RankTo: from.Add(24 * time.Hour), Grain: "day", Page: 1, PageSize: 1})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 || len(stats.Items) != 1 || len(stats.Buckets) != 1 || len(stats.Ranking) != 1 || stats.Ranking[0].Count != 2 {
		t.Fatalf("stats %#v", stats)
	}
	if stats.Items[0].Title != "Example" {
		t.Fatal("deletion lost playback snapshot")
	}
}

func TestPlaybackEventSessionBoundaries(t *testing.T) {
	s := NewSessionTrackerService(zap.NewNop())
	now := time.Now()
	s.now = func() time.Time { return now }
	first := s.PlaybackEventSession("u", "d", "m", "", false)
	if next := s.PlaybackEventSession("u", "d", "m", "", false); next != first {
		t.Fatal("heartbeat creates another session")
	}
	if stopped := s.PlaybackEventSession("u", "d", "m", "", true); stopped != first {
		t.Fatal("stop lost session identity")
	}
	second := s.PlaybackEventSession("u", "d", "m", "", false)
	if second == first {
		t.Fatal("new playback reused stopped session")
	}
	now = now.Add(31 * time.Minute)
	if next := s.PlaybackEventSession("u", "d", "m", "", false); next == second {
		t.Fatal("idle timeout reused session")
	}
}
