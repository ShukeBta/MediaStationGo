package service

import (
	"math"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyUserDataUsesRecordedCompletionAndDurationEverywhere(t *testing.T) {
	for _, tc := range []struct {
		name       string
		runtime    int
		history    model.PlaybackHistory
		played     bool
		percentage float64
	}{
		{"short complete", 120, model.PlaybackHistory{PositionMs: 90_000, DurationMs: 120_000, Completed: true}, true, 75},
		{"legacy short threshold", 120, model.PlaybackHistory{PositionMs: 90_000, DurationMs: 120_000}, true, 75},
		{"selected longer version", 120, model.PlaybackHistory{PositionMs: 120_000, DurationMs: 240_000}, false, 50},
		{"selected shorter version", 240, model.PlaybackHistory{PositionMs: 90_000, DurationMs: 120_000}, true, 75},
		{"explicit completion", 120, model.PlaybackHistory{PositionMs: 20_000, DurationMs: 120_000, Completed: true}, true, 100.0 / 6},
		{"completed unknown runtime", 0, model.PlaybackHistory{Completed: true}, true, 100},
		{"legacy manual marker after probe", 3600, model.PlaybackHistory{PositionMs: 1, DurationMs: 1, Completed: true}, true, 100},
		{"missing recorded runtime", 120, model.PlaybackHistory{PositionMs: 90_000}, true, 75},
		{"unwatched tiny clip", 20, model.PlaybackHistory{}, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newTestEmbyService(t)
			lib := model.Library{Name: "Completion", Path: t.TempDir(), Type: "movie", Enabled: true}
			if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
				t.Fatal(err)
			}
			viewer := &model.User{Username: "viewer", Role: "user", IsActive: true}
			if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
				t.Fatal(err)
			}
			setTestUserLibraries(t, svc, viewer, lib.ID)
			media := &model.Media{LibraryID: lib.ID, Title: "Completion", Path: lib.Path + "/movie.mkv", DurationSec: tc.runtime}
			if err := svc.repo.DB.Create(media).Error; err != nil {
				t.Fatal(err)
			}
			history := tc.history
			history.UserID, history.MediaID, history.WatchedAt = viewer.ID, media.ID, time.Now()
			if err := svc.repo.DB.Create(&history).Error; err != nil {
				t.Fatal(err)
			}
			check := func(source string, data map[string]any) {
				t.Helper()
				percentage, _ := data["PlayedPercentage"].(float64)
				if data["Played"] != tc.played || math.Abs(percentage-tc.percentage) > 0.001 {
					t.Errorf("%s user data = %+v, want played=%t percentage=%g", source, data, tc.played, tc.percentage)
				}
			}
			data, found, err := svc.MediaUserData(t.Context(), viewer.ID, media.ID)
			if err != nil || !found {
				t.Fatalf("action payload: found=%t err=%v", found, err)
			}
			check("action", data)
			item, err := svc.Item(t.Context(), media.ID, viewer.ID)
			if err != nil || item == nil {
				t.Fatalf("item: %v", err)
			}
			check("detail", item["UserData"].(map[string]any))
			items, err := svc.payloadsForMediaRows(t.Context(), []model.Media{*media}, viewer.ID, true, false)
			if err != nil || len(items) != 1 {
				t.Fatalf("list: items=%d err=%v", len(items), err)
			}
			check("list", items[0]["UserData"].(map[string]any))
			resume, err := svc.ResumeItems(t.Context(), viewer.ID)
			if err != nil {
				t.Fatal(err)
			}
			resuming := resume["Items"].([]map[string]any)
			if tc.played || history.PositionMs <= 0 {
				if len(resuming) != 0 {
					t.Errorf("completed/unwatched item remains in resume: %+v", resuming)
				}
			} else if len(resuming) != 1 {
				t.Errorf("in-progress item missing from resume: %+v", resume)
			} else {
				check("resume", resuming[0]["UserData"].(map[string]any))
			}
		})
	}
}

func TestEmbyManualPlayedUnknownDurationSurvivesRuntimeProbe(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Manual played", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{Username: "viewer", Role: "user", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	setTestUserLibraries(t, svc, viewer, lib.ID)
	media := &model.Media{LibraryID: lib.ID, Title: "Unknown runtime", Path: lib.Path + "/manual.mkv"}
	if err := svc.repo.DB.Create(media).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.MarkPlayed(t.Context(), viewer.ID, media.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.repo.DB.Model(media).Update("duration_sec", 3600).Error; err != nil {
		t.Fatal(err)
	}
	var history model.PlaybackHistory
	if err := svc.repo.DB.Where("media_id = ?", media.ID).First(&history).Error; err != nil {
		t.Fatal(err)
	}
	if !embyHistoryRowFullyPlayed(history) {
		t.Fatal("manual completed status lost after probe")
	}
	if data, found, err := svc.MediaUserData(t.Context(), viewer.ID, media.ID); err != nil || !found || data["Played"] != true {
		t.Fatalf("manual completed payload lost after probe: %+v, %t, %v", data, found, err)
	}
	// An older resumable row must not override the latest explicit completion.
	older := model.PlaybackHistory{UserID: viewer.ID, MediaID: media.ID, PositionMs: 60_000, DurationMs: 3_600_000, WatchedAt: history.WatchedAt.Add(-time.Hour)}
	if err := svc.repo.DB.Create(&older).Error; err != nil {
		t.Fatal(err)
	}
	if resume, err := svc.ResumeItems(t.Context(), viewer.ID); err != nil || len(resume["Items"].([]map[string]any)) != 0 {
		t.Fatalf("older unfinished row overrode manual completion: %+v, %v", resume, err)
	}
	if err := svc.MarkPlayed(t.Context(), viewer.ID, media.ID, false); err != nil {
		t.Fatal(err)
	}
	var count int64
	svc.repo.DB.Model(&model.PlaybackHistory{}).Where("media_id = ?", media.ID).Count(&count)
	if count != 0 {
		t.Fatal("marking unplayed did not clear manual completion")
	}
}

func TestEmbyNextUpAdvancesAfterShortClipCompletion(t *testing.T) {
	episodes := []model.Media{{Base: model.Base{ID: "episode-1"}, DurationSec: 120}, {Base: model.Base{ID: "episode-2"}, DurationSec: 120}}
	history := map[string]model.PlaybackHistory{
		"episode-1": {MediaID: "episode-1", PositionMs: 90_000, DurationMs: 120_000, WatchedAt: time.Now()},
	}
	got, ok := nextUpCandidateForEpisodes(episodes, history, true)
	if !ok || got.episode.ID != "episode-2" {
		t.Fatalf("next up reused completed short episode: %+v, %t", got, ok)
	}
}
