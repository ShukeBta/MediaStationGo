package handler

import (
	"context"
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
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type embyRouteCloudResolver struct {
	link  *cloud.DirectLink
	typ   string
	ref   string
	ua    string
	calls int
}

func (r *embyRouteCloudResolver) CloudResolve(_ context.Context, typ, ref, ua string) (*cloud.DirectLink, error) {
	r.typ = typ
	r.ref = ref
	r.ua = ua
	r.calls++
	return r.link, nil
}

func TestEmbyVideoStreamRoutesUseConfiguredSTRMMode(t *testing.T) {
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
	stream := service.NewStreamService(cfg, zap.NewNop(), repos, nil)
	resolver := &embyRouteCloudResolver{link: &cloud.DirectLink{URL: "https://video.example.test/Movie.mkv?sign=test"}}
	stream.SetCloudProbe(resolver)
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:   repos,
		Emby:   service.NewEmbyService(cfg, zap.NewNop(), repos),
		Stream: stream,
	})

	token := signedTestToken(t, secret)
	paths := []string{
		"/Videos/cloud-1/stream",
		"/Videos/cloud-1/stream.mkv",
		"/Videos/cloud-1/original",
		"/Videos/cloud-1/original.mkv",
		"/videos/cloud-1/stream",
		"/videos/cloud-1/stream.mkv",
		"/videos/cloud-1/original",
		"/videos/cloud-1/original.mkv",
		"/emby/Videos/cloud-1/stream",
		"/emby/Videos/cloud-1/stream.mkv",
		"/emby/Videos/cloud-1/original",
		"/emby/Videos/cloud-1/original.mkv",
		"/emby/videos/cloud-1/stream",
		"/emby/videos/cloud-1/stream.mkv",
		"/emby/videos/cloud-1/original",
		"/emby/videos/cloud-1/original.mkv",
	}
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		for _, path := range paths {
			t.Run(method+" "+path, func(t *testing.T) {
				before := resolver.calls
				req := httptest.NewRequest(method, path+"?api_key="+token, nil)
				req.Header.Set("User-Agent", "GenericEmbyClient/1.0")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)

				if w.Code != http.StatusFound {
					t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
				}
				if loc := w.Header().Get("Location"); loc != resolver.link.URL {
					t.Fatalf("stream route should resolve directly with configured STRM mode, got %q", loc)
				}
				if resolver.calls != before+1 || resolver.typ != "openlist" || resolver.ref != "/Movies/Movie.mkv" || resolver.ua != "GenericEmbyClient/1.0" {
					t.Fatalf("unexpected cloud resolve call: %#v", resolver)
				}
				if got := w.Header().Get("Cache-Control"); !strings.Contains(got, "no-store") {
					t.Fatalf("stream redirect Cache-Control = %q, want no-store", got)
				}
			})
		}
	}
}

func TestEmbyLegacyAPIStreamUsesConfiguredRedirectProxyMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackModeSettingKey, service.CloudPlaybackModeRedirectProxy); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackSTRMEnabledSettingKey, "true"); err != nil {
		t.Fatalf("enable strm playback: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), service.CloudPlaybackRedirectEnabledSettingKey, "true"); err != nil {
		t.Fatalf("enable redirect playback: %v", err)
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
	cfg := &config.Config{Secrets: config.SecretsConfig{JWTSecret: secret}}
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:   repos,
		Emby:   service.NewEmbyService(cfg, zap.NewNop(), repos),
		Stream: service.NewStreamService(cfg, zap.NewNop(), repos, nil),
	})

	token := signedTestToken(t, secret)
	for _, method := range []string{http.MethodGet, http.MethodHead} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/emby/api/stream/cloud-1?api_key="+token, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusFound {
				t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
			}
			loc := w.Header().Get("Location")
			if !strings.Contains(loc, "/api/cloud/play/openlist?") || !strings.Contains(loc, "token=") {
				t.Fatalf("legacy stream route should use configured redirect/proxy mode, got %q", loc)
			}
			if strings.Contains(loc, "/api/stream/cloud-1") {
				t.Fatalf("legacy route must not force STRM mode: %q", loc)
			}
		})
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
