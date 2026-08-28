package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestExternalIDPresenceReturnsOnlyVisibleRequestedIDs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	viewer := &model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", PasswordHash: "hash", Role: "admin", HideAdult: true}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	safe := model.Library{Base: model.Base{ID: "safe"}, Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	adult := model.Library{Base: model.Base{ID: "adult"}, Name: "成人", Path: "/media/adult", Type: "adult", Enabled: true}
	if err := db.Create(&[]model.Library{safe, adult}).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "safe-media"}, LibraryID: safe.ID, Title: "可见", Path: "/media/movies/visible.mkv", TMDbID: 101, DoubanID: "db-visible"},
		{Base: model.Base{ID: "adult-media"}, LibraryID: adult.ID, Title: "隐藏", Path: "/media/adult/hidden.mkv", TMDbID: 202, DoubanID: "db-hidden", NSFW: true},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos, Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos)}

	body, err := json.Marshal(externalIDPresenceRequest{
		TMDbIDs:   []int{101, 202, 404, 101},
		DoubanIDs: []string{"db-visible", "db-hidden", "db-missing", "db-visible"},
	})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set(middleware.CtxUserID, viewer.ID)
	c.Set(middleware.CtxUserRole, viewer.Role)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/media/external-id-presence", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	externalIDPresenceHandler(svc)(c)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var response externalIDPresenceResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if want := []int{101}; !reflect.DeepEqual(response.TMDbIDs, want) {
		t.Fatalf("tmdb_ids=%v want=%v", response.TMDbIDs, want)
	}
	if want := []string{"db-visible"}; !reflect.DeepEqual(response.DoubanIDs, want) {
		t.Fatalf("douban_ids=%v want=%v", response.DoubanIDs, want)
	}
}

func TestExternalIDPresenceRejectsInvalidAndOversizedBatches(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := externalIDPresenceHandler(&service.Container{})
	for _, requestBody := range []externalIDPresenceRequest{
		{TMDbIDs: []int{0}},
		{DoubanIDs: []string{""}},
		{TMDbIDs: func() []int {
			ids := make([]int, maxExternalIDPresenceBatch+1)
			for index := range ids {
				ids[index] = index + 1
			}
			return ids
		}()},
	} {
		body, err := json.Marshal(requestBody)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/media/external-id-presence", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		handler(c)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("body=%s status=%d want=%d", body, w.Code, http.StatusBadRequest)
		}
	}
}
