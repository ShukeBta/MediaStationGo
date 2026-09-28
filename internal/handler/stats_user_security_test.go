package handler

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func TestStatsPlayPreservesResumeAndUsesProgressRules(t *testing.T) {
	cfg, svc, user := currentRoleTestServices(t)
	if err := svc.Repo.DB.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	svc.Media = service.NewMediaService(cfg, svc.Log, svc.Repo)
	svc.Playback = service.NewPlaybackService(svc.Log, svc.Repo)
	svc.Sessions = service.NewSessionTrackerService(svc.Log)
	for _, lib := range []string{"public", "private"} {
		if err := svc.Repo.DB.Create(&model.Library{Base: model.Base{ID: lib}, Name: lib, Path: "/media/" + lib, Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Repo.User.UpdateFields(t.Context(), user.ID, map[string]any{"role": "user", "allowed_library_ids": `["public"]`}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []model.Media{
		{Base: model.Base{ID: "movie"}, LibraryID: "public", Title: "Movie", Path: "/media/public/movie.mkv", DurationSec: 1200},
		{Base: model.Base{ID: "secret"}, LibraryID: "private", Title: "Secret", Path: "/media/private/movie.mkv", DurationSec: 1200},
	} {
		if err := svc.Repo.DB.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	original := model.PlaybackHistory{UserID: user.ID, MediaID: "movie", PositionMs: 360000, DurationMs: 1200000, Completed: false, WatchedAt: time.Now().Add(-time.Hour)}
	if err := svc.Repo.DB.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	token, err := svc.Auth.IssueToken(user)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	Register(r, cfg, svc.Log, svc)
	currentRoleRequest(t, r, token, "POST", "/api/stats/play", `{"media_id":"movie","completed":true}`, 200)
	var saved model.PlaybackHistory
	if err := svc.Repo.DB.First(&saved, "id = ?", original.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.PositionMs != original.PositionMs || saved.DurationMs != original.DurationMs || saved.Completed || !saved.WatchedAt.Equal(original.WatchedAt) {
		t.Fatalf("bare event changed resume: %#v", saved)
	}
	currentRoleRequest(t, r, token, "POST", "/api/stats/play", `{"media_id":"movie","position_ms":600000,"completed":true,"session_id":"watch"}`, 200)
	if err := svc.Repo.DB.First(&saved, "id = ?", original.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.PositionMs != 600000 || saved.DurationMs != 1200000 || saved.Completed {
		t.Fatalf("explicit progress bypassed shared rules: %#v", saved)
	}
	for _, body := range []string{
		`{"media_id":"movie","position_ms":-1}`, `{"media_id":"movie","position_ms":600000,"duration_ms":-1}`,
	} {
		currentRoleRequest(t, r, token, "POST", "/api/stats/play", body, 400)
	}
	currentRoleRequest(t, r, token, "POST", "/api/stats/play", `{"media_id":"secret","position_ms":600000}`, 404)
	currentRoleRequest(t, r, token, "POST", "/api/stats/play", `{"media_id":"movie","position_ms":1080000,"completed":false}`, 200)
	if err := svc.Repo.DB.First(&saved, "id = ?", original.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !saved.Completed {
		t.Fatal("client overrode server completion threshold")
	}
}
