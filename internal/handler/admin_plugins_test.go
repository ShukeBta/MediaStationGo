package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func pluginRouteFixture(t *testing.T) (*gin.Engine, *service.Container, string) {
	t.Helper()
	cfg, svc, user := currentRoleTestServices(t)
	if err := svc.Repo.DB.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	svc.Plugins = service.NewPluginService(svc.Repo)
	token, err := svc.Auth.IssueToken(user)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	Register(router, cfg, svc.Log, svc)
	return router, svc, token
}

func pluginResponseObject(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var object map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &object); err != nil {
		t.Fatal(err)
	}
	return object
}

func TestAdminPluginRoutesLifecycle(t *testing.T) {
	router, svc, token := pluginRouteFixture(t)
	const path = "/api/admin/plugins/library-summary"
	list := pluginResponseObject(t, currentRoleRequest(t, router, token, http.MethodGet, "/api/admin/plugins", "", http.StatusOK))
	items, ok := list["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one built-in plugin, got %#v", list)
	}
	initial := items[0].(map[string]any)
	if initial["id"] != "library-summary" || initial["enabled"] != false || initial["status"] != "disabled" || initial["last_run"] != nil {
		t.Fatalf("unexpected initial plugin: %#v", initial)
	}
	currentRoleRequest(t, router, token, http.MethodPost, path+"/run", "", http.StatusConflict)
	updated := pluginResponseObject(t, currentRoleRequest(t, router, token, http.MethodPatch, path,
		`{"enabled":true,"config":{"include_disabled":true}}`, http.StatusOK))
	configuration, ok := updated["config"].(map[string]any)
	if !ok || configuration["include_disabled"] != true || updated["enabled"] != true || updated["status"] != "ready" {
		t.Fatalf("configuration was not applied: %#v", updated)
	}
	result := pluginResponseObject(t, currentRoleRequest(t, router, token, http.MethodPost, path+"/run", "", http.StatusOK))
	metrics, ok := result["metrics"].([]any)
	if !ok || len(metrics) == 0 || result["summary"] == "" || result["completed_at"] == "" || result["duration_ms"] == nil {
		t.Fatalf("missing run result fields: %#v", result)
	}
	refreshed := pluginResponseObject(t, currentRoleRequest(t, router, token, http.MethodGet, "/api/admin/plugins", "", http.StatusOK))
	if refreshed["items"].([]any)[0].(map[string]any)["last_run"] == nil {
		t.Fatal("refresh lost the last run result")
	}
	// Configuration survives restarts; execution records are scoped to this process.
	svc.Plugins = service.NewPluginService(svc.Repo)
	persisted := pluginResponseObject(t, currentRoleRequest(t, router, token, http.MethodGet, "/api/admin/plugins", "", http.StatusOK))
	plugin := persisted["items"].([]any)[0].(map[string]any)
	if plugin["enabled"] != true || plugin["config"].(map[string]any)["include_disabled"] != true || plugin["last_run"] != nil {
		t.Fatalf("plugin state was not persisted: %#v", plugin)
	}
	currentRoleRequest(t, router, token, http.MethodPatch, path, `{"enabled":false}`, http.StatusOK)
	currentRoleRequest(t, router, token, http.MethodPost, path+"/run", "", http.StatusConflict)
}

func TestAdminPluginRoutesRejectInvalidUpdates(t *testing.T) {
	router, _, token := pluginRouteFixture(t)
	for _, tc := range []struct{ name, body string }{
		{"wrong config type", `{"config":{"include_disabled":"true"}}`},
		{"unknown config field", `{"config":{"unexpected":true}}`},
		{"unknown top-level field", `{"enabled":true,"unexpected":true}`},
		{"empty update", `{}`},
		{"null update", `null`},
		{"multiple JSON values", `{"enabled":true} {"enabled":false}`},
		{"oversized body", `{"config":{"include_disabled":"` + strings.Repeat("x", 17<<10) + `"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			currentRoleRequest(t, router, token, http.MethodPatch, "/api/admin/plugins/library-summary", tc.body, http.StatusBadRequest)
		})
	}
	currentRoleRequest(t, router, token, http.MethodPatch, "/api/admin/plugins/unknown", `{"enabled":true}`, http.StatusNotFound)
	currentRoleRequest(t, router, token, http.MethodPost, "/api/admin/plugins/unknown/run", "", http.StatusNotFound)
	// Rejected payloads must not partially enable the plugin.
	currentRoleRequest(t, router, token, http.MethodPost, "/api/admin/plugins/library-summary/run", "", http.StatusConflict)
}

func TestAdminPluginRoutesRequireCurrentAdminRole(t *testing.T) {
	for _, identity := range []string{"anonymous", "ordinary user", "demoted admin"} {
		t.Run(identity, func(t *testing.T) {
			cfg, svc, user := currentRoleTestServices(t)
			svc.Plugins = service.NewPluginService(svc.Repo)
			var token string
			var err error
			if identity == "demoted admin" {
				token, err = svc.Auth.IssueToken(user)
				if err != nil {
					t.Fatal(err)
				}
			}
			if identity != "anonymous" {
				if err := svc.Repo.User.UpdateFields(t.Context(), user.ID, map[string]any{"role": "user"}); err != nil {
					t.Fatal(err)
				}
				if identity == "ordinary user" {
					user.Role = "user"
					token, err = svc.Auth.IssueToken(user)
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			router := gin.New()
			Register(router, cfg, svc.Log, svc)
			want := http.StatusForbidden
			if identity == "anonymous" {
				want = http.StatusUnauthorized
			}
			for _, route := range []struct{ method, path, body string }{
				{http.MethodGet, "/api/admin/plugins", ""},
				{http.MethodPatch, "/api/admin/plugins/library-summary", `{"enabled":true}`},
				{http.MethodPost, "/api/admin/plugins/library-summary/run", ""},
			} {
				request := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
				request.Header.Set("Content-Type", "application/json")
				if token != "" {
					request.Header.Set("Authorization", "Bearer "+token)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != want {
					t.Fatalf("%s %s: status=%d want=%d body=%s", route.method, route.path, response.Code, want, response.Body.String())
				}
			}
		})
	}
}

func TestAdminSettingsCannotWritePluginState(t *testing.T) {
	router, svc, token := pluginRouteFixture(t)
	for _, key := range []string{"plugins.library-summary", "plugins.library-summary.enabled", "plugins.library-summary.config"} {
		body, err := json.Marshal(map[string]string{"key": key, "value": `{"enabled":true}`})
		if err != nil {
			t.Fatal(err)
		}
		currentRoleRequest(t, router, token, http.MethodPut, "/api/admin/settings", string(body), http.StatusBadRequest)
		var count int64
		if err := svc.Repo.DB.Model(&model.Setting{}).Where("key = ?", key).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("reserved setting %q persisted through generic settings API", key)
		}
	}
}
