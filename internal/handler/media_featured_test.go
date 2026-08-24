package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestWeeklyFeaturedHandlerHonorsUserLibraryScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Media{}, &model.WeeklyFeaturedSelection{}, &model.Setting{}, &model.PlayProfile{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	allowed := model.Library{Base: model.Base{ID: "allowed"}, Name: "允许媒体库", Path: "/media/allowed", Type: "movie", Enabled: true}
	denied := model.Library{Base: model.Base{ID: "denied"}, Name: "禁止媒体库", Path: "/media/denied", Type: "movie", Enabled: true}
	if err := db.Create(&[]model.Library{allowed, denied}).Error; err != nil {
		t.Fatal(err)
	}
	user := model.User{
		Base: model.Base{ID: "viewer"}, Username: "viewer", PasswordHash: "hash", Role: "user", IsActive: true,
		AllowedLibraryIDs: []string{allowed.ID},
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Media{
		{Base: model.Base{ID: "allowed-item"}, LibraryID: allowed.ID, Title: "允许的高分作品", Path: "/media/allowed/work/main.mkv", Rating: 8.8},
		{Base: model.Base{ID: "denied-item"}, LibraryID: denied.ID, Title: "无权限高分作品", Path: "/media/denied/work/main.mkv", Rating: 10},
	}).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos, Media: service.NewMediaService(&config.Config{}, zap.NewNop(), repos)}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(middleware.CtxUserID, user.ID)
	c.Set(middleware.CtxUserRole, user.Role)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/media/featured", nil)
	weeklyFeaturedMediaHandler(svc)(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Item *service.SeriesCard `json:"item"`
		Week string              `json:"week"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Item == nil || payload.Item.Rep.ID != "allowed-item" {
		t.Fatalf("featured item escaped user scope: %#v", payload.Item)
	}
	if payload.Item.Rep.Path != "" || payload.Item.LinkMedia.Path != "" {
		t.Fatalf("non-admin response leaked storage paths: %#v", payload.Item)
	}
}
