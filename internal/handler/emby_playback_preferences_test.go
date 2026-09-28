package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbyPlaybackPreferencePersistsPartialUpdatesPerUser(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserMediaPlaybackPreference{}); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repository.New(db)}
	newRouter := func(userID string) *gin.Engine {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(middleware.CtxUserID, userID)
			c.Next()
		})
		router.GET("/Items/:id/PlaybackPreferences", embyPlaybackPreferenceHandler(svc))
		router.PUT("/Items/:id/PlaybackPreferences", embyUpdatePlaybackPreferenceHandler(svc))
		return router
	}

	userOne := newRouter("user-1")
	initial := getEmbyPlaybackPreference(t, userOne, "media-1")
	if initial.Configured || !initial.SubtitleEnabled || initial.SubtitleTrackKey != nil || initial.AudioTrackKey != nil {
		t.Fatalf("initial preference = %#v", initial)
	}

	putEmbyPlaybackPreference(t, userOne, "media-1", `{"subtitle_enabled":true,"subtitle_track_key":"stream:2"}`)
	putEmbyPlaybackPreference(t, userOne, "media-1", `{"audio_track_key":"stream:1"}`)
	putEmbyPlaybackPreference(t, userOne, "media-1", `{"subtitle_enabled":false}`)
	preference := getEmbyPlaybackPreference(t, userOne, "media-1")
	if !preference.Configured || preference.SubtitleEnabled || valueOf(preference.SubtitleTrackKey) != "stream:2" || valueOf(preference.AudioTrackKey) != "stream:1" {
		t.Fatalf("user-1 preference = %#v", preference)
	}

	other := getEmbyPlaybackPreference(t, newRouter("user-2"), "media-1")
	if other.Configured || !other.SubtitleEnabled || other.SubtitleTrackKey != nil || other.AudioTrackKey != nil {
		t.Fatalf("user-2 preference = %#v", other)
	}
}

func TestEmbyPlaybackPreferenceRejectsEmptyTrackKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserMediaPlaybackPreference{}); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repository.New(db)}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(middleware.CtxUserID, "user-1")
		c.Next()
	})
	router.PUT("/Items/:id/PlaybackPreferences", embyUpdatePlaybackPreferenceHandler(svc))

	req := httptest.NewRequest(http.MethodPut, "/Items/media-1/PlaybackPreferences", bytes.NewBufferString(`{"audio_track_key":" "}`))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func putEmbyPlaybackPreference(t *testing.T, router http.Handler, mediaID, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/Items/"+mediaID+"/PlaybackPreferences", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func getEmbyPlaybackPreference(t *testing.T, router http.Handler, mediaID string) embyPlaybackPreferenceResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/Items/"+mediaID+"/PlaybackPreferences", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var preference embyPlaybackPreferenceResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &preference); err != nil {
		t.Fatal(err)
	}
	return preference
}

func valueOf(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
