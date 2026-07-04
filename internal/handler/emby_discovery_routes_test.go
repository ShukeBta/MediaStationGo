package handler

import (
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

func TestEmbyWithRequestAddressUsesHost(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://192.168.1.4:18080/System/Info/Public", nil)

	payload := embyWithRequestAddress(c, map[string]any{"Id": "mediastation-go-001"})

	if payload["LocalAddress"] != "http://192.168.1.4:18080" {
		t.Fatalf("unexpected LocalAddress: %#v", payload["LocalAddress"])
	}
	if payload["WanAddress"] != "http://192.168.1.4:18080" {
		t.Fatalf("unexpected WanAddress: %#v", payload["WanAddress"])
	}
}

func TestEmbyWithRequestAddressHonorsForwardedHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/System/Info/Public", nil)
	c.Request.Header.Set("X-Forwarded-Proto", "https")
	c.Request.Header.Set("X-Forwarded-Host", "media.example.test")

	payload := embyWithRequestAddress(c, map[string]any{"Id": "mediastation-go-001"})

	if payload["LocalAddress"] != "https://media.example.test" {
		t.Fatalf("unexpected LocalAddress: %#v", payload["LocalAddress"])
	}
}

func TestEmbyPublicSystemInfoLooksLikeModernEmbyServer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	repos := repository.New(db)
	router := gin.New()
	registerEmbyRoutes(router, "test-secret", &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})

	req := httptest.NewRequest(http.MethodGet, "/System/Info/Public", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode system info: %v", err)
	}
	if payload["ProductName"] != "Emby Server" {
		t.Fatalf("ProductName = %#v, want Emby Server", payload["ProductName"])
	}
	version, _ := payload["Version"].(string)
	if !strings.HasPrefix(version, "4.") {
		t.Fatalf("Version = %q, want Emby-compatible 4.x", version)
	}
}

func TestEmbyMobileCompatibilityRoutesAvoidPlaybackBlocking404s(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "viewer",
		PasswordHash: "hash",
		Role:         "admin",
		IsActive:     true,
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	cfg := &config.Config{}
	cfg.Secrets.JWTSecret = "test-secret"
	repos := repository.New(db)
	router := gin.New()
	registerEmbyRoutes(router, cfg.Secrets.JWTSecret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(cfg, zap.NewNop(), repos),
	})

	token := signedTestToken(t, cfg.Secrets.JWTSecret)
	tests := []struct {
		path     string
		auth     bool
		wantCode int
	}{
		{path: "/emby/System/Ext/ServerDomains", wantCode: http.StatusOK},
		{path: "/emby/Items/msgo-series-demo/Similar", auth: true, wantCode: http.StatusOK},
		{path: "/emby/api/danmu/media-demo/raw", auth: true, wantCode: http.StatusOK},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		if tc.auth {
			req.Header.Set("X-Emby-Token", token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.wantCode {
			t.Fatalf("%s status = %d body=%s", tc.path, w.Code, w.Body.String())
		}
	}
}

func TestEmbyOfficialClientProbeRoutesAvoidHomepageBlocking404s(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
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

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
	})
	token := signedTestToken(t, secret)
	tests := []struct {
		method string
		path   string
		auth   bool
	}{
		{method: http.MethodGet, path: "/emby/CustomCssJS/Scripts"},
		{method: http.MethodGet, path: "/emby/Localization/cultures"},
		{method: http.MethodPost, path: "/emby/Sessions/Logout"},
		{method: http.MethodGet, path: "/emby/System/WakeOnLanInfo", auth: true},
		{method: http.MethodGet, path: "/emby/ScheduledTasks", auth: true},
		{method: http.MethodGet, path: "/emby/LiveTv/Recordings", auth: true},
		{method: http.MethodGet, path: "/emby/System/ActivityLog/Entries", auth: true},
		{method: http.MethodGet, path: "/emby/web/configurationpages", auth: true},
		{method: http.MethodPost, path: "/emby/Users/user-1/Configuration", auth: true},
		{method: http.MethodGet, path: "/emby/Items/Latest?UserId=user-1", auth: true},
		{method: http.MethodGet, path: "/emby/Items/Resume?UserId=user-1", auth: true},
		{method: http.MethodGet, path: "/emby/Genres", auth: true},
		{method: http.MethodGet, path: "/emby/Shows/Upcoming", auth: true},
		{method: http.MethodGet, path: "/emby/Items/item-1/ThumbnailSet", auth: true},
		{method: http.MethodGet, path: "/emby/Items/item-1/ThemeMedia", auth: true},
		{method: http.MethodGet, path: "/emby/Users/user-1/Items/item-1/SpecialFeatures", auth: true},
		{method: http.MethodGet, path: "/emby/Users/user-1/Items/item-1/Intros", auth: true},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.auth {
				req.Header.Set("X-Emby-Token", token)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code == http.StatusNotFound {
				t.Fatalf("route returned 404 body=%s", w.Body.String())
			}
			if w.Code >= 500 {
				t.Fatalf("route returned %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestEmbySenPlayerDiscoveryRoutesReturnProtocolResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	cfg := &config.Config{}
	cfg.App.Port = 9011
	repos := repository.New(db)
	router := gin.New()
	registerEmbyRoutes(router, "test-secret", &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(cfg, zap.NewNop(), repos),
	})

	tests := []struct {
		path        string
		contentType string
		contains    string
	}{
		{path: "/emby", contentType: "application/json", contains: "Emby Server"},
		{path: "/emby/", contentType: "application/json", contains: "Emby Server"},
		{path: "/Startup/Configuration", contentType: "application/json", contains: "StartupWizardCompleted"},
		{path: "/emby/Startup/Configuration", contentType: "application/json", contains: "StartupWizardCompleted"},
		{path: "/System/Configuration/Public", contentType: "application/json", contains: "IsStartupWizardCompleted"},
		{path: "/emby/System/Configuration/Public", contentType: "application/json", contains: "IsStartupWizardCompleted"},
		{path: "/QuickConnect/Enabled", contentType: "application/json", contains: "false"},
		{path: "/emby/QuickConnect/Enabled", contentType: "application/json", contains: "false"},
		{path: "/Branding/Css", contentType: "text/css", contains: ""},
		{path: "/emby/Branding/Css", contentType: "text/css", contains: ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
			}
			if contentType := w.Header().Get("Content-Type"); !strings.Contains(contentType, tt.contentType) {
				t.Fatalf("Content-Type = %q, want %q", contentType, tt.contentType)
			}
			if tt.contains != "" && !strings.Contains(w.Body.String(), tt.contains) {
				t.Fatalf("body = %q, want contains %q", w.Body.String(), tt.contains)
			}
			if strings.Contains(w.Body.String(), "<html") {
				t.Fatalf("protocol discovery route served SPA HTML: %q", w.Body.String())
			}
		})
	}
}
