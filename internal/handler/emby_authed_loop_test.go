package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

// TestEmbyLoginThenAuthedRequest simulates the exact flow an Emby-compatible
// client performs: POST /Users/AuthenticateByName to obtain an AccessToken,
// then immediately reuses that token to fetch /Users/Me and /Items.
//
// This guards against regressions where login succeeds but the returned token
// fails downstream auth (the "每次登录后立刻 401 Invalid token" report).
func TestEmbyLoginThenAuthedRequest(t *testing.T) {
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
	tokenSvc := service.NewTokenService(cfg, log, repos)
	auth := service.NewAuthService(cfg, log, repos, tokenSvc, permissions)
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

	loginBody := `{"Username":"viewer","Pw":"secret-pass"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/emby/Users/AuthenticateByName", strings.NewReader(loginBody))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", w.Code, w.Body.String())
	}
	var login struct {
		AccessToken string `json:"AccessToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	if login.AccessToken == "" {
		t.Fatalf("empty AccessToken from login")
	}

	// Reuse the returned token against authenticated endpoints.
	for _, path := range []string{"/emby/Users/Me", "/emby/Items", "/emby/Library/VirtualFolders"} {
		w = httptest.NewRecorder()
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Emby-Token", login.AccessToken)
		router.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s with returned token = %d body=%s", path, w.Code, w.Body.String())
		}
	}
}

// TestEmbyLoginWithAuthorizationHeaderToken covers clients that place the
// bearer token in Authorization instead of X-Emby-Token.
func TestEmbyLoginWithAuthorizationHeaderToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		// :memory: SQLite 每个连接是独立库，必须锁单连接否则表在不同连接上互相不可见。
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

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/emby/Users/AuthenticateByName", strings.NewReader(`{"Username":"viewer","Pw":"secret-pass"}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login status = %d body=%s", w.Code, w.Body.String())
	}
	var login struct {
		AccessToken string `json:"AccessToken"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &login); err != nil {
		t.Fatalf("decode login: %v", err)
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/emby/Users/Me", nil)
	req.Header.Set("Authorization", "MediaBrowser Token=\""+login.AccessToken+"\"")
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Authorization-token request = %d body=%s", w.Code, w.Body.String())
	}
}