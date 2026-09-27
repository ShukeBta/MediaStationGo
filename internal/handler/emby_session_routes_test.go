package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyMarkPlayedRefreshesPlaybackDevice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
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
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := repos.DB.Create(&model.Media{
		Base:      model.Base{ID: "media-1"},
		LibraryID: lib.ID,
		Title:     "Watched Movie",
		Path:      `/media/movies/Watched Movie.mkv`,
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:   repos,
		Emby:   service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
		Device: service.NewDeviceService(zap.NewNop(), repos),
	})

	token := signedTestToken(t, secret)
	req := httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/PlayedItems/media-1", nil)
	req.Header.Set("X-MediaBrowser-Authorization", `MediaBrowser Client="Infuse", Device="iPhone", DeviceId="played-device", Token="`+token+`"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	devices, err := repos.UserDevice.ListByUser(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 1 || devices[0].LastPlayAt == nil {
		t.Fatalf("mark played should refresh playback device, got %#v", devices)
	}
	if devices[0].DeviceID != "played-device" || devices[0].DeviceName != "iPhone" || devices[0].Client != "Infuse" {
		t.Fatalf("playback device info not parsed: %#v", devices[0])
	}
}

func TestEmbyProgressDoesNotPersistUntilCloudResolveSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
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
	lib := model.Library{Base: model.Base{ID: "library-1"}, Name: "Cloud", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:        model.Base{ID: "media-1"},
		LibraryID:   lib.ID,
		Title:       "Cloud Movie",
		Path:        "cloud://openlist/Movies/Movie.mkv",
		STRMURL:     "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv",
		DurationSec: 120,
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	log := zap.NewNop()
	playback := service.NewPlaybackService(log, repos)
	emby := service.NewEmbyService(&config.Config{}, log, repos)
	emby.SetPlaybackService(playback)
	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{Repo: repos, Emby: emby, Playback: playback, Log: log})

	sendProgress := func() *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/emby/Sessions/Playing/Progress", strings.NewReader(`{"ItemId":"media-1","PositionTicks":300000000,"RunTimeTicks":1200000000}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}

	if w := sendProgress(); w.Code != http.StatusNoContent {
		t.Fatalf("unresolved progress status = %d body=%s", w.Code, w.Body.String())
	}
	var count int64
	if err := db.Model(&model.PlaybackHistory{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("history count = %d before successful resolve", count)
	}

	playback.AuthorizeResolvedCloudPlayback("user-1", media.ID)
	if w := sendProgress(); w.Code != http.StatusNoContent {
		t.Fatalf("authorized progress status = %d body=%s", w.Code, w.Body.String())
	}
	if err := db.Model(&model.PlaybackHistory{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("history count = %d after successful resolve", count)
	}
}

func TestEmbyProgressUsesSelectedCloudMediaSourceButPersistsLogicalItem(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
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
	lib := model.Library{Base: model.Base{ID: "library-1"}, Name: "Cloud", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := []model.Media{
		{
			Base: model.Base{ID: "version-primary"}, LibraryID: lib.ID, Title: "特立独行",
			Path: "cloud://openlist/Movies/Keep.Real.DV.mp4", STRMURL: "/api/cloud/play/openlist?ref=%2FMovies%2FKeep.Real.DV.mp4",
			VersionGroupKey: "keep-real", DurationSec: 120, Width: 3840, SizeBytes: 30_000,
		},
		{
			Base: model.Base{ID: "version-selected"}, LibraryID: lib.ID, Title: "特立独行",
			Path: "cloud://openlist/Movies/Keep.Real.HDR.mp4", STRMURL: "/api/cloud/play/openlist?ref=%2FMovies%2FKeep.Real.HDR.mp4",
			VersionGroupKey: "keep-real", DurationSec: 120, Width: 3840, SizeBytes: 20_000,
		},
		{
			Base: model.Base{ID: "unrelated-version"}, LibraryID: lib.ID, Title: "其他影片",
			Path: "cloud://openlist/Movies/Other.mp4", STRMURL: "/api/cloud/play/openlist?ref=%2FMovies%2FOther.mp4",
			VersionGroupKey: "other", DurationSec: 120,
		},
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	log := zap.NewNop()
	playback := service.NewPlaybackService(log, repos)
	emby := service.NewEmbyService(&config.Config{}, log, repos)
	emby.SetPlaybackService(playback)
	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{Repo: repos, Emby: emby, Playback: playback, Log: log})
	token := signedTestToken(t, secret)

	playback.AuthorizeResolvedCloudPlayback("user-1", "version-selected")
	req := httptest.NewRequest(
		http.MethodPost,
		"/emby/Sessions/Playing/Progress",
		strings.NewReader(`{"ItemId":"version-primary","MediaSourceId":"version-selected","PositionTicks":300000000,"RunTimeTicks":1200000000}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Token", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("selected source progress status = %d body=%s", w.Code, w.Body.String())
	}

	var history model.PlaybackHistory
	if err := db.Where("user_id = ? AND media_id = ?", "user-1", "version-primary").First(&history).Error; err != nil {
		t.Fatalf("load logical item history: %v", err)
	}
	if history.PositionMs != 30_000 || history.DurationMs != 120_000 || history.Completed {
		t.Fatalf("logical item history = %#v", history)
	}
	var selectedHistoryCount int64
	if err := db.Model(&model.PlaybackHistory{}).
		Where("user_id = ? AND media_id = ?", "user-1", "version-selected").
		Count(&selectedHistoryCount).Error; err != nil {
		t.Fatal(err)
	}
	if selectedHistoryCount != 0 {
		t.Fatalf("selected source created %d fragmented history rows", selectedHistoryCount)
	}

	resume, err := emby.ResumeItems(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("resume items: %v", err)
	}
	items, _ := resume["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "version-primary" {
		t.Fatalf("resume items = %#v", items)
	}
	userData, _ := items[0]["UserData"].(map[string]any)
	if userData["PlaybackPositionTicks"] != int64(300_000_000) {
		t.Fatalf("resume user data = %#v", userData)
	}

	playback.AuthorizeResolvedCloudPlayback("user-1", "unrelated-version")
	req = httptest.NewRequest(
		http.MethodPost,
		"/emby/Sessions/Playing/Progress",
		strings.NewReader(`{"ItemId":"version-primary","MediaSourceId":"unrelated-version","PositionTicks":600000000,"RunTimeTicks":1200000000}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Emby-Token", token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unrelated source status = %d body=%s", w.Code, w.Body.String())
	}
	if err := db.Where("user_id = ? AND media_id = ?", "user-1", "version-primary").First(&history).Error; err != nil {
		t.Fatal(err)
	}
	if history.PositionMs != 30_000 {
		t.Fatalf("unrelated source changed logical progress to %d", history.PositionMs)
	}
}

func TestEmbyHideFromResumeReturnsUserDataWithoutClearingProgress(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
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
	lib := model.Library{Name: "Movies", Path: t.TempDir(), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := repos.DB.Create(&model.Media{
		Base:        model.Base{ID: "media-1"},
		LibraryID:   lib.ID,
		Title:       "Resume Movie",
		Path:        lib.Path + "/resume.mp4",
		DurationSec: 120,
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	if err := repos.History.Upsert(t.Context(), &model.PlaybackHistory{
		UserID:     "user-1",
		MediaID:    "media-1",
		PositionMs: 30_000,
		DurationMs: 120_000,
		Completed:  false,
	}); err != nil {
		t.Fatalf("create history: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})

	req := httptest.NewRequest(http.MethodPost, "/emby/Users/user-1/Items/media-1/HideFromResume?Hide=true", nil)
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	var userData map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &userData); err != nil {
		t.Fatalf("decode user data: %v", err)
	}
	if got := int64(userData["PlaybackPositionTicks"].(float64)); got != 30_000*10_000 {
		t.Fatalf("PlaybackPositionTicks = %d, want preserved progress", got)
	}
	var count int64
	if err := db.Model(&model.PlaybackHistory{}).Where("user_id = ? AND media_id = ?", "user-1", "media-1").Count(&count).Error; err != nil {
		t.Fatalf("count history: %v", err)
	}
	if count != 1 {
		t.Fatalf("HideFromResume must not delete playback history, count=%d", count)
	}
}

func TestEmbyCompatSessionAllowsSameClientRequestsWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	permissions := service.NewPermissionService(log, repos)
	auth := service.NewAuthService(cfg, log, repos, service.NewTokenService(cfg, log, repos), permissions)
	user, _, err := auth.Register(context.Background(), "viewer", "secret-pass")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := repos.Library.Create(t.Context(), &model.Library{Name: "Movies", Path: "D:\\media", Type: "movie", Enabled: true}); err != nil {
		t.Fatalf("create library: %v", err)
	}

	embyCompatSessions.Lock()
	embyCompatSessions.items = map[string]embyCompatSession{}
	embyCompatSessions.Unlock()

	router := gin.New()
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, &service.Container{
		Repo:  repos,
		Auth:  auth,
		Emby:  service.NewEmbyService(cfg, log, repos),
		Audit: service.NewAuditService(log, repos),
	})

	req := httptest.NewRequest(http.MethodPost, "/emby/Users/authenticatebyname", strings.NewReader(`{"Username":"viewer","Pw":"secret-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Emby Theater")
	req.Header.Set("X-Emby-Device-Id", "pc-device")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("login status: %d body=%s", w.Code, w.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/emby/Users/"+user.ID+"/Views", nil)
	req.Header.Set("User-Agent", "Emby Theater")
	req.Header.Set("X-Emby-Device-Id", "pc-device")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("views status: %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode views: %v", err)
	}
	if _, ok := payload["Items"]; !ok {
		t.Fatalf("missing Items: %#v", payload)
	}
}

func TestEmbyAuthenticatedRequestRefreshesRealtimeUserActivity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	oldLogin := time.Now().Add(-6 * time.Hour)
	if err := repos.User.Create(t.Context(), &model.User{
		Base:        model.Base{ID: "user-1"},
		Username:    "viewer",
		Role:        "admin",
		Tier:        "plus",
		IsActive:    true,
		LastLoginAt: &oldLogin,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	const secret = "test-secret"
	log := zap.NewNop()
	tracker := service.NewSessionTrackerService(log)
	svc := &service.Container{
		Repo:     repos,
		Emby:     service.NewEmbyService(&config.Config{}, log, repos),
		Sessions: tracker,
	}
	router := gin.New()
	registerEmbyRoutes(router, secret, svc)
	router.GET("/admin/users", listUsersHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/emby/Users/Me", nil)
	req.Header.Set("X-MediaBrowser-Authorization", `MediaBrowser Client="Infuse", Device="iPhone", DeviceId="phone-1", Token="`+signedTestToken(t, secret)+`"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("me status: %d body=%s", w.Code, w.Body.String())
	}

	sessions := tracker.List(t.Context())
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v, want one realtime session", sessions)
	}
	if sessions[0].UserID != "user-1" || sessions[0].DeviceID != "phone-1" || sessions[0].Client != "Infuse" {
		t.Fatalf("session did not capture client info: %#v", sessions[0])
	}

	req = httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("admin users status: %d body=%s", w.Code, w.Body.String())
	}
	var users []model.User
	if err := json.Unmarshal(w.Body.Bytes(), &users); err != nil {
		t.Fatalf("decode users: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("users = %#v", users)
	}
	if users[0].LastLoginAt == nil || !users[0].LastLoginAt.After(oldLogin) {
		t.Fatalf("last_login_at = %v, want realtime value after %v", users[0].LastLoginAt, oldLogin)
	}
	if !users[0].RealtimeOnline || users[0].RealtimeDeviceCount != 1 {
		t.Fatalf("realtime flags online=%v devices=%d", users[0].RealtimeOnline, users[0].RealtimeDeviceCount)
	}
}

func TestEmbySessionCapabilitiesRefreshesRealtimeActivity(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPost, "/emby/Sessions/Capabilities/Full?deviceId=phone-1&device=iPhone&client=Infuse", strings.NewReader(`{}`))
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("capabilities status: %d body=%s", w.Code, w.Body.String())
	}
	sessions := tracker.List(t.Context())
	if len(sessions) != 1 {
		t.Fatalf("sessions = %#v, want one heartbeat session", sessions)
	}
	if sessions[0].DeviceID != "phone-1" || sessions[0].DeviceName != "iPhone" || sessions[0].Client != "Infuse" || sessions[0].UserName != "viewer" {
		t.Fatalf("capabilities did not refresh client session: %#v", sessions[0])
	}
}

func TestEmbySessionCapabilitiesIgnoresScopedPlaybackToken(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPost, "/emby/Sessions/Capabilities/Full?deviceId=phone-1&device=iPhone&client=Infuse", strings.NewReader(`{}`))
	req.Header.Set("X-Emby-Token", signedTestTokenWithPurpose(t, secret, service.ExternalPlaybackTokenPurpose))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("capabilities status: %d body=%s", w.Code, w.Body.String())
	}
	if sessions := tracker.List(t.Context()); len(sessions) != 0 {
		t.Fatalf("scoped external playback token must not create realtime session: %#v", sessions)
	}
}

func TestEmbyLogoutRemovesRealtimeSessionFromPublicRoute(t *testing.T) {
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
	tracker.RecordActivity(t.Context(), "user-1", "viewer", "phone-1", "iPhone", "Infuse", "192.0.2.10")
	tracker.RecordActivity(t.Context(), "user-1", "viewer", "tv-1", "Apple TV", "Yamby", "192.0.2.11")
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:     repos,
		Sessions: tracker,
	})

	req := httptest.NewRequest(http.MethodPost, "/emby/Sessions/Logout", nil)
	req.Header.Set("X-MediaBrowser-Authorization", `MediaBrowser Client="Infuse", Device="iPhone", DeviceId="phone-1", Token="`+signedTestToken(t, secret)+`"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("logout status: %d body=%s", w.Code, w.Body.String())
	}
	sessions := tracker.List(t.Context())
	if len(sessions) != 1 {
		t.Fatalf("sessions after logout = %#v, want only the other device", sessions)
	}
	if sessions[0].DeviceID != "tv-1" {
		t.Fatalf("remaining session = %#v, want tv-1", sessions[0])
	}
}

func TestEmbyLogoutIgnoresScopedPlaybackToken(t *testing.T) {
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
	tracker.RecordActivity(t.Context(), "user-1", "viewer", "phone-1", "iPhone", "Infuse", "192.0.2.10")
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:     repos,
		Sessions: tracker,
	})

	req := httptest.NewRequest(http.MethodPost, "/emby/Sessions/Logout", nil)
	req.Header.Set("X-MediaBrowser-Authorization", `MediaBrowser Client="Infuse", Device="iPhone", DeviceId="phone-1", Token="`+signedTestTokenWithPurpose(t, secret, service.ExternalPlaybackTokenPurpose)+`"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("logout status: %d body=%s", w.Code, w.Body.String())
	}
	sessions := tracker.List(t.Context())
	if len(sessions) != 1 || sessions[0].DeviceID != "phone-1" {
		t.Fatalf("scoped external playback token must not logout realtime session: %#v", sessions)
	}
}

func TestEmbyUppercaseSessionCapabilitiesRouteNoContent(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodPost, "/Sessions/Capabilities/Full", strings.NewReader(`{}`))
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
}

func TestEmbySessionCapabilitiesRouteAllowsPreAuthProbe(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerEmbyRoutes(router, "test-secret", &service.Container{})

	for _, path := range []string{"/Sessions/Capabilities", "/Sessions/Capabilities/Full", "/emby/Sessions/Capabilities", "/emby/Sessions/Capabilities/Full"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusNoContent {
				t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
