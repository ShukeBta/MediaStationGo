package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestStorageConfigHandlersRejectQuark(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.PUT("/admin/storage/:type", saveStorageConfigHandler(nil))

	req := httptest.NewRequest(http.MethodPut, "/admin/storage/quark", strings.NewReader(`{"type":"quark","config":{"cookie":"x"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unsupported storage type") {
		t.Fatalf("body = %s, want unsupported storage type", w.Body.String())
	}
}

func TestStorageConfigCloudTestSuccessSavesAndEnablesProvider(t *testing.T) {
	gin.SetMode(gin.TestMode)
	openlist := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/list" {
			t.Fatalf("unexpected openlist path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "openlist-token" {
			t.Fatalf("authorization = %q, want openlist-token", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 200,
			"data": map[string]any{"content": []any{}, "total": 0},
		})
	}))
	defer openlist.Close()

	db, err := gorm.Open(sqlite.Open("file:storage_config_cloud_test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.StorageConfig{}, &model.Setting{}, &model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	log := zap.NewNop()
	storage := service.NewStorageConfigService(log, repos, service.NewCryptoService("", log))
	enabled := false
	if _, err := storage.Save(t.Context(), service.StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": openlist.URL,
			"token":  "openlist-token",
		},
		Enabled: &enabled,
	}); err != nil {
		t.Fatal(err)
	}

	router := gin.New()
	router.POST("/admin/storage/:type/test", testStorageConfigHandler(&service.Container{StorageCfg: storage}))
	body := `{"type":"openlist","config":{"server":"` + openlist.URL + `","token":"openlist-token"}}`
	req := httptest.NewRequest(http.MethodPost, "/admin/storage/openlist/test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("body = %s, want ok true", w.Body.String())
	}
	view, err := storage.Get(t.Context(), "openlist")
	if err != nil {
		t.Fatal(err)
	}
	if view == nil || !view.Enabled {
		t.Fatalf("openlist enabled = %#v, want enabled after successful test", view)
	}
	if _, err := storage.CloudProvider(t.Context(), "openlist"); err != nil {
		t.Fatalf("cloud provider after test: %v", err)
	}
}
