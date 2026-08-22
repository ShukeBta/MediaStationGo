package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestAuthenticatedRouteSurfacesAreRegistered(t *testing.T) {
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
		"GET /api/me",
		"GET /api/auth/permissions",
		"GET /api/libraries",
		"PATCH /api/libraries/:id",
		"GET /api/media",
		"POST /api/media/probes/missing",
		"GET /api/stream/:id",
		"GET /api/storage",
		"GET /api/downloads",
		"GET /api/subscriptions",
		"GET /api/sites/search",
		"GET /api/watch-history",
		"GET /api/discover/preferences",
		"PUT /api/discover/preferences",
		"GET /api/discover/feed",
		"GET /api/discover/items/:source/:provider_id",
		"GET /api/playback/:id/info",
		"GET /api/download/tasks",
		"GET /api/admin/assistant/history",
	} {
		if !routes[want] {
			t.Fatalf("%s route is not registered", want)
		}
	}
}

func TestSubscriptionRoutesRejectNonAdminUsers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	authed := router.Group("/api")
	authed.Use(func(c *gin.Context) {
		c.Set(middleware.CtxUserRole, "user")
		c.Next()
	})
	registerAuthedSubscriptionRoutes(authed, &service.Container{})
	registerAuthedSubscriptionExtraRoutes(authed, &service.Container{})

	for _, request := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/subscriptions"},
		{http.MethodPost, "/api/subscriptions"},
		{http.MethodPut, "/api/subscriptions/subscription-id"},
		{http.MethodDelete, "/api/subscriptions/subscription-id/history"},
		{http.MethodPost, "/api/subscriptions/subscription-id/run"},
	} {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(request.method, request.path, nil)
		router.ServeHTTP(recorder, req)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("%s %s status = %d, want 403", request.method, request.path, recorder.Code)
		}
	}
}
