package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestDeleteUserRefusesRecentRealtimeSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	admin := model.User{Base: model.Base{ID: "admin"}, Username: "admin", PasswordHash: "x", Role: "admin", IsActive: true}
	viewer := model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.DB.Create(&[]model.User{admin, viewer}).Error; err != nil {
		t.Fatal(err)
	}
	tracker := service.NewSessionTrackerService(zap.NewNop())
	tracker.RecordLogin(t.Context(), viewer.ID, viewer.Username, "dev-1", "Apple TV", "Yamby", "10.0.0.8")
	svc := &service.Container{Repo: repos, Sessions: tracker}
	router := gin.New()
	router.DELETE("/admin/users/:id", deleteUserHandler(svc))

	req := httptest.NewRequest(http.MethodDelete, "/admin/users/viewer", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if found, _ := repos.User.FindByID(t.Context(), viewer.ID); found == nil {
		t.Fatal("recent realtime user should not be deleted")
	}
}

func TestUpdateUserLibrariesPersistsValidatedScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	viewer := model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(t.Context(), &viewer); err != nil {
		t.Fatal(err)
	}
	allowed := model.Library{Base: model.Base{ID: "library-allowed"}, Name: "电影", Path: "/media/movies", Enabled: true}
	if err := repos.Library.Create(t.Context(), &allowed); err != nil {
		t.Fatal(err)
	}

	svc := &service.Container{Repo: repos}
	router := gin.New()
	router.PUT("/admin/users/:id/libraries", updateUserLibrariesHandler(svc))

	body, _ := json.Marshal(map[string]any{"allowed_library_ids": []string{" " + allowed.ID + " ", allowed.ID}})
	req := httptest.NewRequest(http.MethodPut, "/admin/users/viewer/libraries", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	updated, err := repos.User.FindByID(t.Context(), viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.AllowedLibraryIDs) != 1 || updated.AllowedLibraryIDs[0] != allowed.ID {
		t.Fatalf("allowed libraries = %#v", updated.AllowedLibraryIDs)
	}

	badBody := []byte(`{"allowed_library_ids":["missing-library"]}`)
	badReq := httptest.NewRequest(http.MethodPut, "/admin/users/viewer/libraries", bytes.NewReader(badBody))
	badReq.Header.Set("Content-Type", "application/json")
	badW := httptest.NewRecorder()
	router.ServeHTTP(badW, badReq)
	if badW.Code != http.StatusBadRequest {
		t.Fatalf("invalid library status = %d body=%s", badW.Code, badW.Body.String())
	}

	emptyBody := []byte(`{"allowed_library_ids":[]}`)
	emptyReq := httptest.NewRequest(http.MethodPut, "/admin/users/viewer/libraries", bytes.NewReader(emptyBody))
	emptyReq.Header.Set("Content-Type", "application/json")
	emptyW := httptest.NewRecorder()
	router.ServeHTTP(emptyW, emptyReq)
	if emptyW.Code != http.StatusOK {
		t.Fatalf("empty library status = %d body=%s", emptyW.Code, emptyW.Body.String())
	}
	updated, err = repos.User.FindByID(t.Context(), viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.AllowedLibraryIDs) != 0 {
		t.Fatalf("cleared allowed libraries = %#v, want none", updated.AllowedLibraryIDs)
	}
}

func TestUpdateUserAdultContentPersistsAdministratorBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	viewer := model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	if err := repos.User.Create(t.Context(), &viewer); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.PATCH("/admin/users/:id/adult-content", updateUserAdultContentHandler(&service.Container{Repo: repos}))
	req := httptest.NewRequest(http.MethodPatch, "/admin/users/viewer/adult-content", bytes.NewBufferString(`{"blocked":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	updated, err := repos.User.FindByID(t.Context(), viewer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.AdultContentBlocked {
		t.Fatal("administrator adult content block was not persisted")
	}
}

func TestAdminCanListSpecifiedUserPlaybackHistory(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Media{}, &model.PlaybackHistory{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	viewer := model.User{Base: model.Base{ID: "viewer-history"}, Username: "viewer", PasswordHash: "x", Role: "user", IsActive: true}
	media := model.Media{Base: model.Base{ID: "adult-media"}, LibraryID: "mixed", Title: "成人作品", Path: "/mixed/adult.mkv", NSFW: true}
	if err := repos.User.Create(t.Context(), &viewer); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	history := model.PlaybackHistory{UserID: viewer.ID, MediaID: media.ID, PositionMs: 12_000, DurationMs: 60_000, WatchedAt: time.Now()}
	if err := repos.DB.Create(&history).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos, Playback: service.NewPlaybackService(zap.NewNop(), repos)}
	router := gin.New()
	router.GET("/admin/users/:id/history", listUserHistoryHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/admin/users/viewer-history/history?page=1&page_size=20", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var payload struct {
		Items []service.HistoryItem `json:"items"`
		Total int64                 `json:"total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Total != 1 || len(payload.Items) != 1 || payload.Items[0].Media == nil || payload.Items[0].Media.ID != media.ID {
		t.Fatalf("unexpected history payload: %#v", payload)
	}
}
