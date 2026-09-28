package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func TestStartupHTTPStatusAndWriteGuardsNeedNoDatabase(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &service.Container{Startup: service.NewStartupState()}
	r := gin.New()
	r.GET("/tasks/startup", taskStartupHandler(svc))
	r.POST("/scheduler/:name/run", schedulerRunHandler(svc))
	r.POST("/libraries/:id/scan", scanLibraryHandler(svc))
	r.POST("/libraries/:id/roots/:root_id/scan", scanLibraryRootHandler(svc))
	r.POST("/media/probes/missing", probeMissingMediaHandler(svc))
	r.POST("/media/:id/tmdb-catalog/refresh", tmdbSeriesCatalogHandler(svc, true))
	for _, path := range []string{"/scheduler/library_scan/run", "/libraries/test/scan", "/libraries/test/roots/root/scan", "/media/probes/missing", "/media/test/tmdb-catalog/refresh"} {
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodPost, path, nil))
		if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), "startup_not_ready") {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
	}
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/tasks/startup", nil))
	var status service.StartupStatus
	if err := json.Unmarshal(response.Body.Bytes(), &status); err != nil || status.State != "starting" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("status: %#v %v", status, err)
	}
}
