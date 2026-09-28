package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func TestEmbyPlaybackPermissionCoversEveryRouteVariant(t *testing.T) {
	cfg, svc, user := currentRoleTestServices(t)
	if err := svc.Repo.User.UpdateFields(t.Context(), user.ID, map[string]any{"role": "user"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Permissions.Save(t.Context(), user.ID, &model.UserPermission{CanPlayMedia: false}); err != nil {
		t.Fatal(err)
	}
	token, err := svc.Auth.IssueEmbyToken(user)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	Register(r, cfg, svc.Log, svc)
	for _, prefix := range []string{"", "/emby"} {
		for _, route := range []struct{ method, path string }{
			{"GET", "/Items"}, {"GET", "/Items/movie"}, {"POST", "/Items/movie/PlaybackInfo"},
			{"GET", "/Videos/movie/stream"}, {"HEAD", "/Videos/movie/stream.mkv"},
			{"GET", "/Videos/movie/master.m3u8"}, {"GET", "/Videos/movie/seg_000001.ts"},
			{"GET", "/Videos/movie/0/Subtitles/0/Stream.srt"}, {"POST", "/Sessions/Playing/Progress"},
		} {
			paths := []string{route.path, strings.ToLower(route.path)}
			if strings.Contains(route.path, "/Subtitles/") {
				paths[1] = strings.Replace(route.path, "/Videos/", "/videos/", 1)
			}
			for _, path := range paths {
				currentRoleRequest(t, r, token, route.method, prefix+path, "", http.StatusForbidden)
			}
		}
	}
	for _, path := range []string{"/api/stream/movie", "/emby/api/stream/movie"} {
		currentRoleRequest(t, r, token, "GET", path, "", http.StatusForbidden)
	}
	currentRoleRequest(t, r, token, "POST", "/api/stats/play", `{"media_id":"movie"}`, http.StatusForbidden)
}

func TestDeviceKickEnforcesSignedIdentityAcrossProtocols(t *testing.T) {
	cfg, svc, user := currentRoleTestServices(t)
	if err := svc.Repo.DB.AutoMigrate(&model.UserDevice{}); err != nil {
		t.Fatal(err)
	}
	svc.Device = service.NewDeviceService(svc.Log, svc.Repo)
	svc.Device.RecordLogin(t.Context(), user.ID, "tv", "Living room", "Emby", "127.0.0.1")
	svc.Device.RecordLogin(t.Context(), user.ID, "phone", "Phone", "Emby", "127.0.0.1")
	token, err := svc.Auth.IssueEmbyDeviceToken(user, "tv", "Living room", "Emby")
	if err != nil {
		t.Fatal(err)
	}
	phone, err := svc.Auth.IssueEmbyDeviceToken(user, "phone", "Phone", "Emby")
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := svc.Auth.IssueEmbyToken(user)
	if err != nil {
		t.Fatal(err)
	}
	web, err := svc.Auth.IssueToken(user)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/emby-stream", middleware.EmbyAuthRequired(cfg.Secrets.JWTSecret), activeEmbyUserRequired(svc), func(c *gin.Context) { c.Status(200) })
	r.GET("/api/stream/movie", middleware.AuthRequired(cfg.Secrets.JWTSecret), activeUserRequired(svc), func(c *gin.Context) { c.Status(200) })
	r.GET("/device", middleware.EmbyAuthRequired(cfg.Secrets.JWTSecret), activeEmbyUserRequired(svc), func(c *gin.Context) { c.JSON(200, embyClientInfoFromRequest(c)) })
	spoofed := httptest.NewRequest("GET", "/device", nil)
	spoofed.Header.Set("Authorization", "Bearer "+token)
	spoofed.Header.Set("X-Emby-Authorization", `MediaBrowser DeviceId="phone", Device="Phone", Client="Other"`)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, spoofed)
	var identity embyClientInfo
	if err := json.Unmarshal(w.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if identity.DeviceID != "tv" || identity.DeviceName != "Living room" || identity.Client != "Emby" {
		t.Fatalf("headers replaced signed terminal identity: %+v", identity)
	}
	for _, path := range []string{"/emby-stream", "/api/stream/movie"} {
		currentRoleRequest(t, r, token, "GET", path, "", 200)
	}
	if err := svc.Device.KickDevice(t.Context(), user.ID, "tv"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/emby-stream", "/api/stream/movie"} {
		for _, header := range []string{"", `MediaBrowser DeviceId="phone", Device="Phone", Client="Emby"`} {
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("X-Emby-Authorization", header)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != 401 {
				t.Fatalf("kicked token %s headers=%q: %d %s", path, header, w.Code, w.Body.String())
			}
		}
		currentRoleRequest(t, r, phone, "GET", path, "", 200)
		currentRoleRequest(t, r, legacy, "GET", path, "", 401)
	}
	currentRoleRequest(t, r, web, "GET", "/api/stream/movie", "", 200)
	svc.Device.RecordLogin(t.Context(), user.ID, "tv", "Living room", "Emby", "127.0.0.1")
	fresh, err := svc.Auth.IssueEmbyDeviceToken(user, "tv", "Living room", "Emby")
	if err != nil {
		t.Fatal(err)
	}
	currentRoleRequest(t, r, fresh, "GET", "/emby-stream", "", 200)
}

func TestEmbySessionsOnlyExposeOwnSessionsUnlessAdmin(t *testing.T) {
	cfg, svc, user := currentRoleTestServices(t)
	svc.Sessions = service.NewSessionTrackerService(svc.Log)
	svc.Sessions.RecordPlayback(t.Context(), user.ID, user.Username, "own", "TV", "Emby", "10.0.0.1", "own-movie", 1000, 2000, false)
	svc.Sessions.RecordPlayback(t.Context(), "other", "Other", "other-device", "Phone", "Emby", "10.0.0.2", "private-movie", 1000, 2000, false)
	token, err := svc.Auth.IssueEmbyToken(user)
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	registerEmbyRoutes(r, cfg.Secrets.JWTSecret, svc)
	for _, state := range []struct {
		role  string
		count int
	}{{"admin", 3}, {"user", 2}} {
		if err := svc.Repo.User.UpdateFields(t.Context(), user.ID, map[string]any{"role": state.role}); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{"/Sessions", "/sessions", "/emby/Sessions", "/emby/sessions"} {
			w := currentRoleRequest(t, r, token, "GET", path, "", 200)
			var rows []map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
				t.Fatal(err)
			}
			if len(rows) != state.count {
				t.Fatalf("%s role %s got %s", path, state.role, w.Body.String())
			}
			if state.role == "user" && (rows[0]["UserId"] != user.ID || strings.Contains(w.Body.String(), "private-movie") || strings.Contains(w.Body.String(), "10.0.0.2")) {
				t.Fatalf("foreign session leaked: %s", w.Body.String())
			}
		}
	}
}
