package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestResourceImportCapabilitiesOnAuthenticatedRoutes(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "disabled"
		if enabled {
			name = "enabled"
		}
		t.Run(name, func(t *testing.T) {
			cfg, svc, user := currentRoleTestServices(t)
			user.Role = "user"
			if err := svc.Repo.User.UpdateFields(t.Context(), user.ID, map[string]any{"role": user.Role}); err != nil {
				t.Fatal(err)
			}
			if enabled {
				// No client or repository is installed: reporting the capability
				// must not try to contact the pipeline or query import jobs.
				svc.ResourceImport = &service.ResourceImportService{}
			}
			token, err := svc.Auth.IssueToken(user)
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			Register(router, cfg, svc.Log, svc)
			const path = "/api/resource-imports/capabilities"
			response := currentRoleRequest(t, router, token, http.MethodGet, path, "", http.StatusOK)
			var body map[string]any
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body) != 1 || body["enabled"] != enabled {
				t.Fatalf("capability must contain only its enabled flag: %s", response.Body)
			}
			response = httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous capability status = %d, want 401", response.Code)
			}
			if !enabled {
				// Keep task lookup distinct from the static capability route.
				currentRoleRequest(t, router, token, http.MethodGet, "/api/resource-imports/example-task", "", http.StatusServiceUnavailable)
			}
		})
	}
}

func TestResourceImportCapabilitiesWithoutContainer(t *testing.T) {
	router := gin.New()
	router.GET("/capabilities", resourceImportCapabilitiesHandler(nil))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/capabilities", nil))
	if response.Code != http.StatusOK || response.Body.String() != `{"enabled":false}` {
		t.Fatalf("nil service capability = %d %s", response.Code, response.Body)
	}
}
