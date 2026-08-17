package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestParseEmbyAuthByNameReqAcceptsLowercaseJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/Users/AuthenticateByName", strings.NewReader(`{"username":"alice","password":"secret"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	req, err := parseEmbyAuthByNameReq(c)
	if err != nil {
		t.Fatalf("parseEmbyAuthByNameReq returned error: %v", err)
	}
	if req.Username != "alice" || req.Password != "secret" {
		t.Fatalf("unexpected request: %#v", req)
	}
}

func TestParseEmbyAuthByNameReqAcceptsFormBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/Users/AuthenticateByName", strings.NewReader("Username=bob&Pw=secret"))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	req, err := parseEmbyAuthByNameReq(c)
	if err != nil {
		t.Fatalf("parseEmbyAuthByNameReq returned error: %v", err)
	}
	if req.Username != "bob" || req.Pw != "secret" {
		t.Fatalf("unexpected request: %#v", req)
	}
}

func TestParseEmbyAuthByNameReqAcceptsJSONWithoutContentType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/emby/users/authenticatebyname", strings.NewReader(`{"UserName":"carol","PW":"secret"}`))

	req, err := parseEmbyAuthByNameReq(c)
	if err != nil {
		t.Fatalf("parseEmbyAuthByNameReq returned error: %v", err)
	}
	if req.Username != "carol" || req.Pw != "secret" {
		t.Fatalf("unexpected request: %#v", req)
	}
}

func TestEmbyAuthenticateByNameAcceptsCaseVariantUsernameAndPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserPermission{}, &model.RefreshToken{}, &model.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	permissions := service.NewPermissionService(log, repos)
	auth := service.NewAuthService(cfg, log, repos, service.NewTokenService(cfg, log, repos), permissions)
	if _, _, err := auth.Register(context.Background(), "viewer", "secret-pass"); err != nil {
		t.Fatalf("register: %v", err)
	}

	router := gin.New()
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, &service.Container{
		Repo:  repos,
		Auth:  auth,
		Emby:  service.NewEmbyService(cfg, log, repos),
		Audit: service.NewAuditService(log, repos),
	})

	req := httptest.NewRequest(http.MethodPost, "/emby/users/authenticatebyname", strings.NewReader(`{"Username":"Viewer","Pw":"secret-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["AccessToken"] == "" {
		t.Fatalf("missing AccessToken: %#v", payload)
	}
}

func TestEmbyAuthenticateRecordsMediaBrowserClientInfo(t *testing.T) {
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
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	log := zap.NewNop()
	permissions := service.NewPermissionService(log, repos)
	auth := service.NewAuthService(cfg, log, repos, service.NewTokenService(cfg, log, repos), permissions)
	if _, _, err := auth.Register(context.Background(), "viewer", "secret-pass"); err != nil {
		t.Fatalf("register: %v", err)
	}

	router := gin.New()
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, &service.Container{
		Repo:        repos,
		Auth:        auth,
		Emby:        service.NewEmbyService(cfg, log, repos),
		Device:      service.NewDeviceService(log, repos),
		Audit:       service.NewAuditService(log, repos),
		Permissions: permissions,
	})

	req := httptest.NewRequest(http.MethodPost, "/emby/Users/AuthenticateByName", strings.NewReader(`{"Username":"viewer","Pw":"secret-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-MediaBrowser-Authorization", `MediaBrowser Client="Infuse", Device="PC", DeviceId="device-42"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	user, err := repos.User.FindByUsername(context.Background(), "viewer")
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	devices, err := repos.UserDevice.ListByUser(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("devices = %#v, want one recorded device", devices)
	}
	if devices[0].DeviceID != "device-42" || devices[0].DeviceName != "PC" || devices[0].Client != "Infuse" {
		t.Fatalf("device info not parsed from MediaBrowser header: %#v", devices[0])
	}
}

// TestEmbyUserIdDirectTokenCompatibility covers clients (e.g. 小幻影视 / Hills)
// that send `X-Emby-Token=UserId="<uuid>"` as the auth credential instead of a
// JWT. The server must accept a valid, non-disabled user id in that format and
// let the request through (user validity is enforced by activeEmbyUserRequired).
func TestEmbyUserIdDirectTokenCompatibility(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		// :memory: SQLite 每个连接是独立库，必须锁单连接。
		sqlDB.SetMaxOpenConns(1)
	}
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

	router := gin.New()
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, &service.Container{
		Repo:        repos,
		Auth:        auth,
		Emby:        service.NewEmbyService(cfg, log, repos),
		Device:      service.NewDeviceService(log, repos),
		Audit:       service.NewAuditService(log, repos),
		Permissions: permissions,
	})

	// 1) UserId="<uuid>" direct credential must pass for a valid user.
	req := httptest.NewRequest(http.MethodGet, "/emby/Users/Me?X-Emby-Token="+url.QueryEscape(`UserId="`+user.ID+`"`), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("userId direct token (query) = %d body=%s", w.Code, w.Body.String())
	}

	// 1b) Emby UserId="<uuid>" in X-Emby-Authorization header (RodelPlayer / 小幻影视 style).
	req = httptest.NewRequest(http.MethodGet, "/emby/Users/Me", nil)
	req.Header.Set("X-Emby-Authorization", `Emby UserId="`+user.ID+`", Client="RodelPlayer", Device="WHILETRUE", DeviceId="dev-1", Version="2.2607.7.0"`)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("userId direct token (X-Emby-Authorization header) = %d body=%s", w.Code, w.Body.String())
	}

	// 2) A random / non-existent user id in the same format must be rejected.
	req = httptest.NewRequest(http.MethodGet, "/emby/Users/Me?X-Emby-Token="+url.QueryEscape(`UserId="does-not-exist"`), nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bogus userId direct token = %d, want 401", w.Code)
	}

	// 2b) Non-existent user in X-Emby-Authorization header must be rejected too.
	req = httptest.NewRequest(http.MethodGet, "/emby/Users/Me", nil)
	req.Header.Set("X-Emby-Authorization", `Emby UserId="does-not-exist", Client="RodelPlayer"`)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bogus userId direct token (header) = %d, want 401", w.Code)
	}
}
