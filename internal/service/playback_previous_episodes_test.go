package service

import (
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
	"testing"
	"time"
)

func TestPlaybackProgressThresholds(t *testing.T) {
	for _, tt := range []struct {
		pos, dur          int64
		record, completed bool
	}{
		{19999, 600000, false, false}, {20000, 600000, true, false},
		{59999, 600001, false, false}, {60000, 600001, true, false},
		{540000, 600000, true, true}, {70000, 100000, true, true},
		{20000, 0, true, false},
	} {
		if got := shouldRecordPlaybackProgress(tt.pos, tt.dur); got != tt.record {
			t.Errorf("record(%d,%d)=%v", tt.pos, tt.dur, got)
		}
		if got := playbackCompleted(tt.pos, tt.dur); got != tt.completed {
			t.Errorf("complete(%d,%d)=%v", tt.pos, tt.dur, got)
		}
	}
	for _, values := range [][2]int64{{-1, 100}, {1, -1}, {101, 100}} {
		if validatePlaybackProgress(values[0], values[1]) == nil {
			t.Errorf("accepted invalid %v", values)
		}
	}
}

func TestPlaybackAutoPreviousEpisodesScopesAndPreservesCompleted(t *testing.T) {
	e := newTestEmbyService(t)
	p := NewPlaybackService(zap.NewNop(), e.repo)
	rows := []model.Media{
		{Base: model.Base{ID: "current"}, LibraryID: "lib", SeriesID: "series", SeasonNum: 1, EpisodeNum: 5},
		{Base: model.Base{ID: "previous"}, LibraryID: "lib", SeriesID: "series", SeasonNum: 1, EpisodeNum: 2},
		{Base: model.Base{ID: "complete"}, LibraryID: "lib", SeriesID: "series", SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "adult"}, LibraryID: "lib", SeriesID: "series", SeasonNum: 1, EpisodeNum: 3, NSFW: true},
		{Base: model.Base{ID: "season"}, LibraryID: "lib", SeriesID: "series", SeasonNum: 2, EpisodeNum: 1},
		{Base: model.Base{ID: "library"}, LibraryID: "other", SeriesID: "series", SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "other-series"}, LibraryID: "lib", SeriesID: "other", SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "future"}, LibraryID: "lib", SeriesID: "series", SeasonNum: 1, EpisodeNum: 6},
		{Base: model.Base{ID: "missing"}, LibraryID: "lib", SeriesID: "series", SeasonNum: 1, EpisodeNum: 4},
	}
	for i := range rows {
		rows[i].Title = rows[i].ID
		rows[i].DurationSec = 1200
		if rows[i].ID != "missing" {
			rows[i].Path = "/media/" + rows[i].ID + ".mkv"
		}
	}
	if err := e.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-24 * time.Hour)
	if err := e.repo.History.Upsert(t.Context(), &model.PlaybackHistory{UserID: "viewer", MediaID: "complete", Completed: true, WatchedAt: old, PositionMs: 77, DurationMs: 88}); err != nil {
		t.Fatal(err)
	}
	if err := e.repo.Setting.Set(t.Context(), autoMarkPreviousEpisodesSetting, "true"); err != nil {
		t.Fatal(err)
	}
	visibility := MediaVisibility{AllowedLibraryIDs: []string{"lib"}}
	if err := p.RecordProgressWithVisibility(t.Context(), "viewer", "current", 59999, 1200000, visibility); err != nil {
		t.Fatal(err)
	}
	var count int64
	e.repo.DB.Model(&model.PlaybackHistory{}).Count(&count)
	if count != 1 {
		t.Fatalf("early history count=%d", count)
	}
	if err := p.RecordProgressWithVisibility(t.Context(), "viewer", "current", 1080000, 1200000, visibility); err != nil {
		t.Fatal(err)
	}
	var history []model.PlaybackHistory
	if err := e.repo.DB.Find(&history).Error; err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 {
		t.Fatalf("history=%+v", history)
	}
	for _, h := range history {
		if !h.Completed {
			t.Fatalf("not completed %+v", h)
		}
		if h.MediaID == "complete" && (!h.WatchedAt.Equal(old) || h.PositionMs != 77) {
			t.Fatalf("overwrote completed %+v", h)
		}
	}
	// Disabled setting and Emby use the same recording threshold.
	if err := e.repo.Setting.Set(t.Context(), autoMarkPreviousEpisodesSetting, "false"); err != nil {
		t.Fatal(err)
	}
	if err := e.RecordProgress(t.Context(), "second", "current", 59999*10000, 1200000*10000); err != nil {
		t.Fatal(err)
	}
	e.repo.DB.Model(&model.PlaybackHistory{}).Where("user_id = ?", "second").Count(&count)
	if count != 0 {
		t.Fatal("Emby recorded early progress")
	}
	if err := e.RecordProgress(t.Context(), "second", "current", 1080000*10000, 1200000*10000); err != nil {
		t.Fatal(err)
	}
	e.repo.DB.Model(&model.PlaybackHistory{}).Where("user_id = ?", "second").Count(&count)
	if count != 1 {
		t.Fatalf("disabled inferred %d rows", count)
	}
}
