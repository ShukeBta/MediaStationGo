package handler

import (
	"encoding/json"
	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPlaybackProgressAndEventsShareAccessRuntimeAndInference(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	user := model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", PasswordHash: "x", Role: "user", Tier: "plus", IsActive: true, AllowedLibraryIDs: []string{"public"}}
	if err = db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"public", "private"} {
		if err = repo.Library.Create(t.Context(), &model.Library{Base: model.Base{ID: id}, Name: id, Path: "/media/" + id, Type: "movie", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	rows := []model.Media{
		{Base: model.Base{ID: "forbidden"}, LibraryID: "private", DurationSec: 120},
		{Base: model.Base{ID: "web"}, LibraryID: "public", DurationSec: 1200},
		{Base: model.Base{ID: "logical"}, LibraryID: "public", DurationSec: 120, VersionGroupKey: "versions"},
		{Base: model.Base{ID: "selected"}, LibraryID: "public", DurationSec: 1200, VersionGroupKey: "versions"},
		{Base: model.Base{ID: "episode1"}, LibraryID: "public", DurationSec: 1200, SeriesID: "series", SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "episode2"}, LibraryID: "public", DurationSec: 1200, SeriesID: "series", SeasonNum: 1, EpisodeNum: 2},
	}
	for i := range rows {
		rows[i].Title = rows[i].ID
		rows[i].Path = "/media/" + rows[i].ID + ".mkv"
	}
	if err = db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	cfg := &config.Config{}
	playback := service.NewPlaybackService(log, repo)
	emby := service.NewEmbyService(cfg, log, repo)
	emby.SetPlaybackService(playback)
	svc := &service.Container{Repo: repo, Cfg: cfg, Playback: playback, Emby: emby, Log: log, Sessions: service.NewSessionTrackerService(log)}
	r := gin.New()
	const secret = "test-secret"
	registerEmbyRoutes(r, secret, svc)
	r.POST("/web/:id", func(c *gin.Context) { c.Set(middleware.CtxUserID, "user-1"); c.Next() }, playbackProgressHandler(svc))
	token := signedTestToken(t, secret)
	post := func(path string, payload any) int {
		data, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(data)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Emby-Token", token)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}
	counts := func(id string, history, event int64) {
		t.Helper()
		var h, e int64
		if err := db.Model(&model.PlaybackHistory{}).Where("media_id = ?", id).Count(&h).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.Model(&model.PlaybackEvent{}).Where("media_id = ?", id).Count(&e).Error; err != nil {
			t.Fatal(err)
		}
		if h != history || e != event {
			t.Fatalf("%s histories/events=%d/%d want %d/%d", id, h, e, history, event)
		}
	}
	for _, isEmby := range []bool{false, true} {
		path := "/web/forbidden"
		payload := map[string]any{"position_ms": 30000, "duration_ms": 120000, "session_id": "forbidden"}
		if isEmby {
			path = "/emby/Sessions/Playing/Progress"
			payload = map[string]any{"ItemId": "forbidden", "PositionTicks": 30000 * 10000, "RunTimeTicks": 120000 * 10000, "PlaySessionId": "forbidden"}
		}
		if code := post(path, payload); code != 404 {
			t.Fatalf("forbidden emby=%v status=%d", isEmby, code)
		}
	}
	counts("forbidden", 0, 0)
	// Hiding the Emby entry is intentionally independent from playback access.
	if err := repo.Setting.Set(t.Context(), service.EmbyLibraryDisplaySettingKey, `[{"id":"public","hidden":true}]`); err != nil {
		t.Fatal(err)
	}
	views, err := emby.Views(t.Context(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if views["TotalRecordCount"] != 0 {
		t.Fatalf("hidden views=%+v", views)
	}
	if code := post("/web/web", map[string]any{"position_ms": 30000, "session_id": "web"}); code != 204 {
		t.Fatal(code)
	}
	counts("web", 0, 0)
	if code := post("/web/web", map[string]any{"position_ms": 60000, "session_id": "web"}); code != 204 {
		t.Fatal(code)
	}
	counts("web", 1, 1)
	for _, position := range []int64{30000, 60000, 90000} {
		if code := post("/emby/Sessions/Playing/Progress", map[string]any{"ItemId": "logical", "MediaSourceId": "selected", "PositionTicks": position * 10000, "PlaySessionId": "version"}); code != 204 {
			t.Fatal(code)
		}
		if position == 30000 {
			counts("logical", 0, 0)
		} else {
			counts("logical", 1, 1)
		}
	}
	counts("selected", 0, 0)
	if err := repo.Setting.Set(t.Context(), "playback.auto_mark_previous_episodes", "true"); err != nil {
		t.Fatal(err)
	}
	if code := post("/emby/Sessions/Playing/Progress", map[string]any{"ItemId": "episode2", "PositionTicks": 1080000 * 10000, "PlaySessionId": "episode"}); code != 204 {
		t.Fatal(code)
	}
	counts("episode2", 1, 1)
	counts("episode1", 1, 0)
}
