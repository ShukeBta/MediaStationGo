package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestEmbyLowercaseVideoStreamRouteServesMedia(t *testing.T) {
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
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "sample.mp4")
	if err := os.WriteFile(mediaPath, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatalf("write media: %v", err)
	}
	lib := model.Library{Name: "电影", Path: dir, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "media-1"},
		LibraryID: lib.ID,
		Title:     "Lowercase Stream",
		Path:      mediaPath,
		Container: "mp4",
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

	req := httptest.NewRequest(http.MethodGet, "/videos/media-1/stream?api_key="+signedTestToken(t, secret), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "fake-video-bytes" {
		t.Fatalf("unexpected stream body: %q", got)
	}
}

func TestEmbyVideoStreamRecordsSelectedSiblingVersion(t *testing.T) {
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
		Base: model.Base{ID: "user-1"}, Username: "tester", PasswordHash: "x", Role: "admin", Tier: "plus", IsActive: true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	dir := t.TempDir()
	primaryPath := filepath.Join(dir, "primary.mp4")
	selectedPath := filepath.Join(dir, "selected.mp4")
	if err := os.WriteFile(primaryPath, []byte("primary-version"), 0o644); err != nil {
		t.Fatalf("write primary media: %v", err)
	}
	if err := os.WriteFile(selectedPath, []byte("selected-version"), 0o644); err != nil {
		t.Fatalf("write selected media: %v", err)
	}
	lib := model.Library{Name: "电影", Path: dir, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := []model.Media{
		{Base: model.Base{ID: "stream-primary"}, LibraryID: lib.ID, Title: "Movie", Path: primaryPath, Container: "mp4", VersionGroupKey: "stream-versions"},
		{Base: model.Base{ID: "stream-selected"}, LibraryID: lib.ID, Title: "Movie", Path: selectedPath, Container: "mp4", VersionGroupKey: "stream-versions"},
		{Base: model.Base{ID: "stream-unrelated"}, LibraryID: lib.ID, Title: "Other", Path: filepath.Join(dir, "other.mp4"), Container: "mp4", VersionGroupKey: "other-versions"},
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	emby := service.NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:   repos,
		Emby:   emby,
		Stream: service.NewStreamService(&config.Config{}, zap.NewNop(), repos, nil),
	})
	token := signedTestToken(t, secret)
	req := httptest.NewRequest(
		http.MethodGet,
		"/videos/stream-selected/stream?api_key="+token,
		nil,
	)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "selected-version" {
		t.Fatalf("selected stream status=%d body=%q", w.Code, w.Body.String())
	}
	detail, err := emby.Item(t.Context(), "stream-primary", "user-1")
	if err != nil {
		t.Fatalf("detail after selected stream: %v", err)
	}
	if sources := detail["MediaSources"].([]map[string]any); sources[0]["Id"] != "stream-selected" {
		t.Fatalf("selected stream should become the preferred source: %#v", sources)
	}

	req = httptest.NewRequest(
		http.MethodGet,
		"/videos/stream-primary/stream?MediaSourceId=stream-unrelated&api_key="+token,
		nil,
	)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("unrelated source status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestEmbyPrefixedAPIStreamRouteServesMedia(t *testing.T) {
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
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "sample.mp4")
	if err := os.WriteFile(mediaPath, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatalf("write media: %v", err)
	}
	lib := model.Library{Name: "电影", Path: dir, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "media-1"},
		LibraryID: lib.ID,
		Title:     "Prefixed API Stream",
		Path:      mediaPath,
		Container: "mp4",
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

	req := httptest.NewRequest(http.MethodGet, "/emby/api/stream/media-1?api_key="+signedTestToken(t, secret), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Body.String(); got != "fake-video-bytes" {
		t.Fatalf("unexpected stream body: %q", got)
	}
}

func TestEmbyLowercaseOriginalHeadRouteServesHeaders(t *testing.T) {
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
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "sample.mp4")
	if err := os.WriteFile(mediaPath, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatalf("write media: %v", err)
	}
	lib := model.Library{Name: "电影", Path: dir, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "media-1"},
		LibraryID: lib.ID,
		Title:     "Lowercase Original",
		Path:      mediaPath,
		Container: "mp4",
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

	req := httptest.NewRequest(http.MethodHead, "/videos/media-1/original.mp4?api_key="+signedTestToken(t, secret), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", w.Code, w.Body.String())
	}
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD response should not include body, got %q", w.Body.String())
	}
}

func TestEmbyItemDownloadRoutesDisabledGlobally(t *testing.T) {
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
		Base: model.Base{ID: "user-1"}, Username: "tester", PasswordHash: "x", Role: "admin", Tier: "plus", IsActive: true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "download.mp4")
	if err := os.WriteFile(mediaPath, []byte("0123456789video"), 0o644); err != nil {
		t.Fatalf("write media: %v", err)
	}
	lib := model.Library{Name: "电影", Path: dir, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base: model.Base{ID: "download-1"}, LibraryID: lib.ID, Title: "Download", Path: mediaPath, Container: "mp4",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
		Stream: service.NewStreamService(&config.Config{}, zap.NewNop(), repos, nil),
	})
	token := signedTestToken(t, secret)
	for _, prefix := range []string{"", "/emby"} {
		for _, path := range []string{"/Items/download-1/Download", "/Users/user-1/Items/download-1/Download"} {
			for _, route := range []string{path, strings.ToLower(path)} {
				for _, method := range []string{http.MethodGet, http.MethodHead} {
					target := prefix + route + "?api_key=" + token
					if method == http.MethodHead {
						target = prefix + route + "?X-Emby-Token=" + token
					}
					request := httptest.NewRequest(method, target, nil)
					request.Header.Set("Range", "bytes=2-5")
					response := httptest.NewRecorder()
					router.ServeHTTP(response, request)
					if response.Code != http.StatusForbidden || response.Body.Len() != 0 {
						t.Fatalf("%s %s: status=%d body=%q", method, target, response.Code, response.Body.String())
					}
				}
			}
		}
	}

	playback := httptest.NewRequest(http.MethodGet, "/emby/Videos/download-1/stream.mp4?api_key="+token, nil)
	playback.Header.Set("Range", "bytes=2-5")
	playbackResponse := httptest.NewRecorder()
	router.ServeHTTP(playbackResponse, playback)
	if playbackResponse.Code != http.StatusPartialContent || playbackResponse.Body.String() != "2345" {
		t.Fatalf("inline playback status=%d body=%q", playbackResponse.Code, playbackResponse.Body.String())
	}
}

func TestEmbyHLSPlaylistCopiesHeaderTokenToSegmentURLs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/emby/Videos/media-1/master.m3u8?profile=mobile", nil)
	c.Request.Header.Set("X-MediaBrowser-Token", "signed-token")
	request := embyHLSPlaylistRequest(c)
	if got := request.URL.Query().Get("api_key"); got != "signed-token" {
		t.Fatalf("segment api_key=%q, want header token", got)
	}
	if got := request.URL.Query().Get("profile"); got != "mobile" {
		t.Fatalf("existing playlist query lost: %q", request.URL.RawQuery)
	}
	if got := c.Request.URL.Query().Get("api_key"); got != "" {
		t.Fatal("HLS token propagation mutated the caller request")
	}
}

func TestEmbyLowercaseVideoHLSRouteDoesNot404WhenDirectOnly(t *testing.T) {
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
	dir := t.TempDir()
	mediaPath := filepath.Join(dir, "sample.mp4")
	if err := os.WriteFile(mediaPath, []byte("fake-video-bytes"), 0o644); err != nil {
		t.Fatalf("write media: %v", err)
	}
	lib := model.Library{Name: "电影", Path: dir, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := db.Create(&model.Media{
		Base:      model.Base{ID: "media-1"},
		LibraryID: lib.ID,
		Title:     "Lowercase HLS",
		Path:      mediaPath,
		Container: "mp4",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	if err := repos.Setting.Set(t.Context(), service.PlaybackDirectOnlySettingKey, "true"); err != nil {
		t.Fatalf("set direct-only: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo:   repos,
		Emby:   service.NewEmbyService(&config.Config{}, zap.NewNop(), repos),
		Stream: service.NewStreamService(&config.Config{}, zap.NewNop(), repos, nil),
	})

	req := httptest.NewRequest(http.MethodGet, "/videos/media-1/master.m3u8?api_key="+signedTestToken(t, secret), nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Fatalf("lowercase HLS route should be registered, got 404")
	}
	if w.Code != http.StatusConflict {
		t.Fatalf("direct-only HLS should return 409, got %d body=%s", w.Code, w.Body.String())
	}
}
