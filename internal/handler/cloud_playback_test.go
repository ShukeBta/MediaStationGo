package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func TestProxyCloudResolvedLinkUsesHEADWithoutSyntheticRange(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var upstreamMethod, upstreamRange string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamMethod = r.Method
		upstreamRange = r.Header.Get("Range")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", "123456")
		w.Header().Set("Accept-Ranges", "bytes")
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodHead, "/api/cloud/play/openlist?ref=movie", nil)

	proxyCloudResolvedLink(cloudPlaybackRequest{
		c:   c,
		typ: "openlist",
		ref: "movie",
		link: &cloud.DirectLink{
			URL:   upstream.URL + "/movie.mp4",
			Proxy: true,
		},
	})

	if upstreamMethod != http.MethodHead {
		t.Fatalf("upstream method = %q, want HEAD", upstreamMethod)
	}
	if upstreamRange != "" {
		t.Fatalf("upstream Range = %q, want empty", upstreamRange)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Content-Length"); got != "123456" {
		t.Fatalf("Content-Length = %q, want full upstream length", got)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD response body length = %d, want 0", rec.Body.Len())
	}
}

func TestCloudPlaybackAuthorizesProgressOnlyAfterSuccessfulResolve(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var getSucceeds bool
	openList := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/fs/list":
			_, _ = w.Write([]byte(`{"code":200,"data":{"content":[],"total":0}}`))
		case "/api/fs/get":
			if !getSucceeds {
				_, _ = w.Write([]byte(`{"code":500,"message":"driver unavailable"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"data":{"raw_url":"https://cdn.example.test/movie.mkv"}}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer openList.Close()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.PlaybackHistory{}, &model.StorageConfig{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := db.Create(&model.User{Base: model.Base{ID: "user-1"}, Username: "user-1", PasswordHash: "x", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Base: model.Base{ID: "library-1"}, Name: "Cloud", Path: "cloud://openlist/Movies", Type: "movie", Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{Base: model.Base{ID: "media-1"}, LibraryID: lib.ID, Title: "Movie", Path: "cloud://openlist/Movies/Movie.mkv"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	log := zap.NewNop()
	storage := service.NewStorageConfigService(log, repos, service.NewCryptoService("test-secret", log))
	if _, err := storage.Save(t.Context(), service.StorageInput{Type: "openlist", Config: map[string]any{"server": openList.URL, "token": "token"}}); err != nil {
		t.Fatal(err)
	}
	playback := service.NewPlaybackService(log, repos)
	svc := &service.Container{Repo: repos, StorageCfg: storage, Playback: playback, Log: log}

	failedRecorder := httptest.NewRecorder()
	failedContext, _ := gin.CreateTestContext(failedRecorder)
	failedContext.Set(middleware.CtxUserID, "user-1")
	failedContext.Set(cloudPlaybackMediaIDContextKey, media.ID)
	failedContext.Request = httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv&media_id=media-1", nil)
	serveCloudResolvedLink(svc, failedContext, "openlist", "/Movies/Movie.mkv")
	if failedRecorder.Code != http.StatusBadGateway {
		t.Fatalf("failed status = %d body=%s", failedRecorder.Code, failedRecorder.Body.String())
	}
	if err := playback.ValidateProgressWrite(t.Context(), "user-1", media.ID); err != service.ErrCloudPlaybackNotResolved {
		t.Fatalf("failed resolve grant error = %v", err)
	}

	getSucceeds = true
	successRecorder := httptest.NewRecorder()
	successContext, _ := gin.CreateTestContext(successRecorder)
	successContext.Set(middleware.CtxUserID, "user-1")
	successContext.Set(cloudPlaybackMediaIDContextKey, media.ID)
	successContext.Request = httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv&media_id=media-1", nil)
	serveCloudResolvedLink(svc, successContext, "openlist", "/Movies/Movie.mkv")
	if successRecorder.Code != http.StatusFound {
		t.Fatalf("success status = %d body=%s", successRecorder.Code, successRecorder.Body.String())
	}
	if err := playback.ValidateProgressWrite(t.Context(), "user-1", media.ID); err != nil {
		t.Fatalf("successful resolve did not authorize progress: %v", err)
	}
}

func TestCloudPlaybackEnforcesAdministratorLibraryAccessForNormalTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	allowed := model.Library{Base: model.Base{ID: "allowed-library"}, Name: "电影", Path: "cloud://openlist/电影", Enabled: true}
	blocked := model.Library{Base: model.Base{ID: "blocked-library"}, Name: "私有库", Path: "cloud://openlist/私有库", Enabled: true}
	if err := db.Create(&[]model.Library{allowed, blocked}).Error; err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{
		Base:              model.Base{ID: "limited-user"},
		Username:          "limited-user",
		PasswordHash:      "hash",
		Role:              "user",
		AllowedLibraryIDs: []string{allowed.ID},
	}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		Base:      model.Base{ID: "blocked-media"},
		LibraryID: blocked.ID,
		Title:     "不可见影片",
		Path:      "cloud://openlist/私有库/blocked.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2F私有库%2Fblocked.mkv",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, viewer.ID)
	c.Set(middleware.CtxUserRole, viewer.Role)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=%2F私有库%2Fblocked.mkv&media_id="+media.ID, nil)

	if enforceScopedCloudPlaybackToken(c, svc, "openlist", "/私有库/blocked.mkv") {
		t.Fatal("normal user token should not resolve media outside administrator library access")
	}
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}

	allowedMedia := model.Media{
		Base:      model.Base{ID: "allowed-media"},
		LibraryID: allowed.ID,
		Title:     "可见影片",
		Path:      "cloud://openlist/电影/allowed.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=%2F电影%2Fallowed.mkv",
	}
	if err := repos.DB.Create(&allowedMedia).Error; err != nil {
		t.Fatal(err)
	}
	allowedRecorder := httptest.NewRecorder()
	allowedContext, _ := gin.CreateTestContext(allowedRecorder)
	allowedContext.Set(middleware.CtxUserID, viewer.ID)
	allowedContext.Set(middleware.CtxUserRole, viewer.Role)
	allowedContext.Request = httptest.NewRequest(http.MethodGet, "/api/cloud/play/openlist?ref=%2F电影%2Fallowed.mkv&media_id="+allowedMedia.ID, nil)
	if !enforceScopedCloudPlaybackToken(allowedContext, svc, "openlist", "/电影/allowed.mkv") {
		t.Fatalf("allowed media was rejected: status=%d body=%s", allowedRecorder.Code, allowedRecorder.Body.String())
	}
}
