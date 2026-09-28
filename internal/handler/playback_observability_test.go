package handler

import (
	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestPlaybackStatsAdminRoutesRequireAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerAdminRoutes(router.Group("/api"), &config.Config{}, &service.Container{})
	for _, path := range []string{"/api/admin/playback-stats", "/api/admin/player-request-logs"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 401 {
			t.Fatalf("%s returned %d", path, response.Code)
		}
	}
}

func TestPlaybackStatsFiltersRejectInvalidParameters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/stats", playbackStatsHandler(&service.Container{}))
	router.GET("/logs", playerRequestLogsHandler(&service.Container{}))
	for _, path := range []string{"/stats?page=0", "/stats?page_size=101", "/stats?from=2026-02-30", "/stats?grain=hour", "/stats?media_type=unknown", "/stats?rank_date=1900-01-01", "/logs?status=600", "/logs?from=2026-02-01&to=2026-01-01"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != 400 {
			t.Fatalf("%s returned %d", path, response.Code)
		}
	}
}
