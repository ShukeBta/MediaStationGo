package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyVirtualFoldersRouteReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	for _, lib := range []model.Library{
		{Name: "电影", Path: "D:\\media\\movies", Type: "movie", Enabled: true},
		{Name: "剧集", Path: "D:\\media\\tv", Type: "tv", Enabled: true},
		{Name: "综艺", Path: "D:\\media\\variety", Type: "variety", Enabled: true},
	} {
		if err := repos.Library.Create(t.Context(), &lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{Repo: repos})

	req := httptest.NewRequest(http.MethodGet, "/Library/VirtualFolders", nil)
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, "application/json") {
		t.Fatalf("expected JSON content type, got %q body=%s", contentType, w.Body.String())
	}
	if strings.HasPrefix(strings.TrimSpace(w.Body.String()), "<!doctype html>") {
		t.Fatalf("route returned frontend HTML instead of JSON")
	}

	var folders []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &folders); err != nil {
		t.Fatalf("decode folders: %v", err)
	}
	if len(folders) != 3 {
		t.Fatalf("expected 3 folders, got %d: %#v", len(folders), folders)
	}
	if folders[1]["CollectionType"] != "tvshows" || folders[2]["CollectionType"] != "tvshows" {
		t.Fatalf("episodic libraries should expose tvshows collection type: %#v", folders)
	}
}

func TestEmbyItemsCountsRouteReturnsJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{Repo: repos})

	for _, path := range []string{"/Items/Counts", "/Users/user-1/Items/Counts", "/items/counts"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", path, w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s decode response: %v", path, err)
		}
		if _, ok := body["MovieCount"]; !ok {
			t.Fatalf("%s missing MovieCount: %#v", path, body)
		}
	}
}

func TestEmbyDisplayPreferencesAllowsAnonymousCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerEmbyRoutes(router, "secret", &service.Container{})

	req := httptest.NewRequest(http.MethodGet, "/emby/DisplayPreferences/usersettings", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected GET status: %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode display preferences: %v", err)
	}
	if body["Id"] != "usersettings" {
		t.Fatalf("unexpected preferences payload: %#v", body)
	}
	customPrefs, ok := body["CustomPrefs"].(map[string]any)
	if !ok {
		t.Fatalf("missing CustomPrefs: %#v", body)
	}
	if customPrefs["homesection0"] != "smalllibrarytiles" || customPrefs["homesection2"] != "none" || customPrefs["latestItems"] != "false" {
		t.Fatalf("homepage sections should expose library tiles without duplicate latest rails: %#v", customPrefs)
	}
	if body["ScrollDirection"] != "Vertical" {
		t.Fatalf("homepage should prefer vertical library browsing, got %#v", body)
	}

	req = httptest.NewRequest(http.MethodPost, "/emby/displaypreferences/usersettings", strings.NewReader(`{}`))
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("unexpected POST status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestEmbyWebSocketRouteUpgradesForOfficialClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerEmbyRoutes(router, "secret", &service.Container{})
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/embywebsocket?api_key=test-token&deviceId=device-1"
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("websocket dial failed status=%d err=%v", status, err)
	}
	defer conn.Close()
	if resp == nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("expected websocket upgrade, got resp=%#v", resp)
	}
}

func TestEmbyWebSocketRefreshesRealtimeActivity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "viewer",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	const secret = "test-secret"
	tracker := service.NewSessionTrackerService(zap.NewNop())
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:     repos,
		Sessions: tracker,
	})
	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/embywebsocket?deviceId=device-1&device=Windows&client=Emby"
	header := http.Header{"X-Emby-Token": []string{signedTestToken(t, secret)}}
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		t.Fatalf("websocket dial failed status=%d err=%v", status, err)
	}
	defer conn.Close()

	sessions := tracker.List(t.Context())
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v, want websocket heartbeat session", sessions)
	}
	if sessions[0].DeviceID != "device-1" || sessions[0].DeviceName != "Windows" || sessions[0].Client != "Emby" {
		t.Fatalf("websocket did not refresh client session: %#v", sessions[0])
	}
}
