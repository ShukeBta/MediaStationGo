package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func TestSTRMTargetRoutesRequireAdminAndPreview(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, role := range []string{"user", "admin"} {
		router := gin.New()
		group := router.Group("/api", func(c *gin.Context) { c.Set(middleware.CtxUserRole, role) })
		registerAuthedMediaRoutes(group, &service.Container{})
		requests := []struct{ method, path string }{{"DELETE", "/api/media/id/strm-target"}}
		if role == "user" {
			requests = append(requests, struct{ method, path string }{"GET", "/api/media/id/strm-target"}, struct{ method, path string }{"GET", "/api/media/id/strm-delete-target"})
		}
		for _, request := range requests {
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(request.method, request.path, strings.NewReader(`{"delete_parent":true}`))
			req.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, req)
			want := http.StatusForbidden
			if role == "admin" {
				want = http.StatusBadRequest
			}
			if recorder.Code != want {
				t.Fatalf("%s %s %s = %d, want %d", role, request.method, request.path, recorder.Code, want)
			}
		}
	}
}
