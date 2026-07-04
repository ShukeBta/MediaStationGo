package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyVideoStreamUsesSTRMWhenRedirectProxyDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackModeSettingKey, service.CloudPlaybackModeSTRM); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackSTRMEnabledSettingKey, "true"); err != nil {
		t.Fatalf("enable strm playback: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackRedirectEnabledSettingKey, "false"); err != nil {
		t.Fatalf("disable redirect playback: %v", err)
	}
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
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "cloud-1"},
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      "cloud://openlist/Movies/Movie.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv",
		Container: "mkv",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: secret}}
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:   repos,
		Emby:   service.NewEmbyService(cfg, zap.NewNop(), repos),
		Stream: service.NewStreamService(cfg, zap.NewNop(), repos, nil),
	})

	token := signedTestToken(t, secret)
	req := httptest.NewRequest(http.MethodGet, "/videos/cloud-1/stream?api_key="+token, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/api/stream/cloud-1") || !strings.Contains(loc, "api_key=") {
		t.Fatalf("STRM mode should redirect /Videos fallback to tokenized /api/stream, got %q", loc)
	}
	if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
		t.Fatalf("STRM fallback redirect Cache-Control = %q, want no-store", got)
	}
	if strings.Contains(loc, "/api/cloud/play/") {
		t.Fatalf("STRM mode should not expose cloud play directly from /Videos fallback: %q", loc)
	}
}

func TestEmbyVideoStreamIssuesTokenForSessionFallbackSTRMRedirect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackModeSettingKey, service.CloudPlaybackModeSTRM); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackSTRMEnabledSettingKey, "true"); err != nil {
		t.Fatalf("enable strm playback: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackRedirectEnabledSettingKey, "false"); err != nil {
		t.Fatalf("disable redirect playback: %v", err)
	}
	user := model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}
	if err := repos.User.Create(t.Context(), &user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "cloud-1"},
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      "cloud://openlist/Movies/Movie.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv",
		Container: "mkv",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: secret}}
	svc := &service.Container{
		Repo:   repos,
		Auth:   service.NewAuthService(cfg, zap.NewNop(), repos, nil, nil),
		Emby:   service.NewEmbyService(cfg, zap.NewNop(), repos),
		Stream: service.NewStreamService(cfg, zap.NewNop(), repos, nil),
	}
	router := gin.New()
	router.GET("/videos/:id/stream", func(c *gin.Context) {
		c.Set(middleware.CtxUserID, user.ID)
		c.Set(middleware.CtxUserRole, user.Role)
		embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy)(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/videos/cloud-1/stream", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/api/stream/cloud-1") || !strings.Contains(loc, "api_key=") {
		t.Fatalf("session fallback redirect should include api_key for /api/stream, got %q", loc)
	}
}

func TestEmbyVideoStreamRedirectKeepsMediaBrowserAuthorizationToken(t *testing.T) {
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
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "cloud-1"},
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      "cloud://openlist/Movies/Movie.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv",
		Container: "mkv",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:   repos,
		Emby:   service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
		Stream: service.NewStreamService(&config.Config{}, zap.NewNop(), repos, nil),
	})

	token := signedTestToken(t, secret)
	req := httptest.NewRequest(http.MethodGet, "/videos/cloud-1/stream", nil)
	req.Header.Set("X-MediaBrowser-Authorization", `MediaBrowser Client="Infuse", Device="PC", Token="`+token+`"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/api/cloud/play/openlist?") || !strings.Contains(loc, "token=") {
		t.Fatalf("redirect Location should target tokenized cloud play endpoint, got %q", loc)
	}
}
