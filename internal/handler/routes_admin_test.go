package handler

import (
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestAdminRouteSurfacesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	Register(router, &config.Config{
		Secrets: config.SecretsConfig{JWTSecret: "test-secret"},
	}, zap.NewNop(), &service.Container{Log: zap.NewNop()})

	routes := map[string]bool{}
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = true
	}

	for _, want := range []string{
		"GET /api/admin/users",
		"GET /api/admin/users/:id/permissions",
		"GET /api/admin/storage/status",
		"GET /api/admin/cloud/:type/list",
		"GET /api/admin/download/clients",
		"POST /api/admin/system/scheduler/:name/trigger",
		"POST /api/admin/backups",
		"GET /api/admin/notify/channels",
		"GET /api/admin/telegram/webhook",
		"GET /api/admin/organize/sources",
		"POST /api/admin/media/repair-rescrape",
		"GET /api/admin/api-configs",
		"POST /api/admin/scheduler/:name/run",
	} {
		if !routes[want] {
			t.Fatalf("%s route is not registered", want)
		}
	}
}
