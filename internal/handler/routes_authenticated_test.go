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
		"GET /System/Info",
		"GET /emby/System/Info",
		"GET /api/me",
		"GET /api/auth/permissions",
		"GET /api/libraries",
		"PATCH /api/libraries/:id",
		"GET /api/media",
		"GET /api/media/featured",
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
	for _, forbidden := range []string{
		"POST /api/auth/register",
		"GET /Users/Public",
		"GET /users/public",
		"GET /System/Info/Public",
		"GET /system/info/public",
		"GET /emby/Users/Public",
		"GET /emby/users/public",
		"GET /emby/System/Info/Public",
		"GET /emby/system/info/public",
	} {
		if routes[forbidden] {
			t.Fatalf("forbidden public route must not be registered: %s", forbidden)
		}
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want 200", response.Code)
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "SAMEORIGIN",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
	} {
		if got := response.Header().Get(header); got != want {
			t.Fatalf("%s = %q, want %q", header, got, want)
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
