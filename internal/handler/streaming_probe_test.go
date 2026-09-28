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
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestReprobeHandlerReturnsUnprocessableEntityForUnprobeableCloudMedia(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	media := model.Media{Title: "Broken Cloud", Path: "cloud://openlist/115/其他/broken.mkv"}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	svc := &service.Container{
		Repo:    repos,
		Stream:  service.NewStreamService(cfg, zap.NewNop(), repos, nil),
		FFprobe: service.NewFFprobeService(cfg, zap.NewNop()),
	}
	router := gin.New()
	router.POST("/media/:id/probe", reprobeHandler(svc))
	req := httptest.NewRequest(http.MethodPost, "/media/"+media.ID+"/probe", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Code  int    `json:"code"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 1 || body.Error == "" {
		t.Fatalf("response = %+v", body)
	}
}
