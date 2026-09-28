package handler

import (
	"encoding/json"
	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbyLibraryDisplaySettingValidationAndViews(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.AutoMigrate(model.AllModels()...); err != nil {
		t.Fatal(err)
	}
	repo := repository.New(db)
	for _, id := range []string{"a", "b", "hidden", "forbidden"} {
		lib := model.Library{Base: model.Base{ID: id}, Name: id, Path: "/" + id, Type: "movie", Enabled: true}
		if err := repo.Library.Create(t.Context(), &lib); err != nil {
			t.Fatal(err)
		}
	}
	user := model.User{Base: model.Base{ID: "viewer"}, Username: "viewer", PasswordHash: "x", Role: "user", AllowedLibraryIDs: []string{"a", "b", "hidden"}}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repo, Cfg: &config.Config{}}
	svc.Emby = service.NewEmbyService(svc.Cfg, zap.NewNop(), repo)
	r := gin.New()
	r.POST("/setting", updateSettingHandler(svc))
	save := func(value string) int {
		data, _ := json.Marshal(settingReq{Key: service.EmbyLibraryDisplaySettingKey, Value: value})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/setting", strings.NewReader(string(data))))
		return w.Code
	}
	for _, value := range []string{`null`, `[{"id":"missing","hidden":false}]`, `[{"id":"a","hidden":"false"}]`} {
		if code := save(value); code != http.StatusBadRequest {
			t.Fatalf("accepted %s status=%d", value, code)
		}
	}
	if code := save(`[{"id":"b","hidden":false},{"id":"hidden","hidden":true},{"id":"a","hidden":false}]`); code != http.StatusNoContent {
		t.Fatalf("save status=%d", code)
	}
	views, err := svc.Emby.Views(t.Context(), user.ID)
	if err != nil {
		t.Fatal(err)
	}
	items := views["Items"].([]map[string]any)
	if len(items) != 2 || items[0]["Id"] != "b" || items[1]["Id"] != "a" {
		t.Fatalf("views=%+v", views)
	}
}
