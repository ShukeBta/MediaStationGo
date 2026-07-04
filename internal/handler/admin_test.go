package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
