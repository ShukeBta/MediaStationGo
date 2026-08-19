package handler

import (
	"encoding/json"
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

// TestEmbySubtitleOfficialRouteServesRawASS verifies that the official Emby
// subtitle route shape
// (/Videos/{itemId}/{mediaSourceId}/Subtitles/{index}/Stream.{format}) is
// registered, resolves the media, and streams the RAW ASS bytes — matching the
// advertised Codec — so a real Emby client can load the subtitle.
func TestEmbySubtitleOfficialRouteServesRawASS(t *testing.T) {
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
	videoPath := filepath.Join(dir, "Movie.mkv")
	subPath := filepath.Join(dir, "Movie.ass")
	rawASS := "Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,test line\n"
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	if err := os.WriteFile(subPath, []byte(rawASS), 0o644); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}

	mediaID := "media-1"
	if err := db.Create(&model.Media{
		Base:       model.Base{ID: mediaID},
		LibraryID:  "lib-1",
		Title:      "Movie",
		Path:       videoPath,
		Container:  "mkv",
		VideoCodec: "h264",
		AudioCodec: "aac",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos).SetSubtitleService(
			service.NewSubtitleService(zap.NewNop(), repos),
		),
	})

	// Official-format route:
	//   /Videos/{Id}/{MediaSourceId}/Subtitles/{Index}/Stream.{Format}
	// Index 2 = first subtitle when audio present (Video 0, Audio 1, Sub 2).
	req := httptest.NewRequest(http.MethodGet, "/emby/Videos/"+mediaID+"/"+mediaID+"/Subtitles/2/Stream.ass", nil)
	req.Header.Set("X-Emby-Token", signedTestToken(t, secret))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want text/plain", ct)
	}
	if got := w.Body.String(); got != rawASS {
		t.Fatalf("served body = %q, want raw ASS %q", got, rawASS)
	}
}

// TestEmbySubtitleDeliveryUrlGetsToken ensures the subtitle DeliveryUrl carries
// the auth token in PlaybackInfo (delivered via embyAttachTokenToMediaSources).
func TestEmbySubtitleDeliveryUrlGetsToken(t *testing.T) {
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
	videoPath := filepath.Join(dir, "Movie.mkv")
	subPath := filepath.Join(dir, "Movie.ass")
	if err := os.WriteFile(videoPath, []byte("video"), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	if err := os.WriteFile(subPath, []byte("Dialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,x\n"), 0o644); err != nil {
		t.Fatalf("write subtitle: %v", err)
	}

	mediaID := "media-1"
	if err := db.Create(&model.Media{
		Base:       model.Base{ID: mediaID},
		LibraryID:  "lib-1",
		Title:      "Movie",
		Path:       videoPath,
		Container:  "mkv",
		VideoCodec: "h264",
		AudioCodec: "aac",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	const secret = "test-secret"
	token := signedTestToken(t, secret)
	router := gin.New()
	registerEmbyRoutes(router, secret, &service.Container{
		Repo: repos,
		Emby: service.NewEmbyService(&config.Config{}, zap.NewNop(), repos).SetSubtitleService(
			service.NewSubtitleService(zap.NewNop(), repos),
		),
	})

	req := httptest.NewRequest(http.MethodGet, "/emby/Items/"+mediaID+"/PlaybackInfo", nil)
	req.Header.Set("X-Emby-Token", token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("unexpected status %d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	source := body["MediaSources"].([]any)[0].(map[string]any)
	streams := source["MediaStreams"].([]any)
	var deliveryURL string
	for _, s := range streams {
		stream := s.(map[string]any)
		if stream["Type"] == "Subtitle" {
			deliveryURL, _ = stream["DeliveryUrl"].(string)
			break
		}
	}
	if deliveryURL == "" {
		t.Fatalf("no subtitle DeliveryUrl found: %#v", source)
	}
	// Official shape with bare MediaSourceId + format suffix and the token appended.
	wantPrefix := "/Videos/" + mediaID + "/" + mediaID + "/Subtitles/2/Stream.ass"
	if deliveryURL[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("DeliveryUrl %q does not start with %q", deliveryURL, wantPrefix)
	}
	if !strings.Contains(deliveryURL, "api_key="+token) {
		t.Fatalf("DeliveryUrl %q missing auth token", deliveryURL)
	}
}
