package handler

import (
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestTaskHistoryRoutesRejectNonAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api")
	group.Use(func(c *gin.Context) { c.Set(middleware.CtxUserRole, "user"); c.Next() })
	registerTaskHistoryRoutes(group, &service.Container{})
	for _, path := range []string{"definitions", "history", "logs", "log-days", "scrape-pending", "definitions/library_scan/run"} {
		method := "GET"
		if path == "definitions/library_scan/run" {
			method = "POST"
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(method, "/api/tasks/"+path, nil))
		if response.Code != 403 {
			t.Fatalf("%s = %d", path, response.Code)
		}
	}
}

func TestTaskHistoryFiltersValidateDateAndPage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/history", taskHistoryHandler(&service.Container{}))
	for _, query := range []string{"?page=-1", "?page_size=500", "?status=unknown", "?from=2026-09-28&to=2026-09-27", "?from=2020-01-01&to=2026-09-28"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest("GET", "/history"+query, nil))
		if response.Code != 400 {
			t.Fatalf("%s = %d", query, response.Code)
		}
	}
}
