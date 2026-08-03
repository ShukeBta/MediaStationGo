package handler

import (
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

func TestPublicUIConfigExposesCommunityLinkVisibility(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), hideCommunityLinksSettingKey, "true"); err != nil {
		t.Fatal(err)
	}
	svc := &service.Container{Repo: repos, Log: zap.NewNop()}
	r := gin.New()
	r.GET("/api/public/ui-config", publicUIConfigHandler(svc))
	req := httptest.NewRequest(http.MethodGet, "/api/public/ui-config", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK || !strings.Contains(resp.Body.String(), `"hide_community_links_for_users":true`) {
		t.Fatalf("status=%d body=%s", resp.Code, resp.Body.String())
	}
}
