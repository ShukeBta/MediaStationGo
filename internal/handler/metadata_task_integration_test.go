package handler

import (
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetadataTaskMutationsRespectStartupGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &service.Container{Startup: service.NewStartupState()}
	r := gin.New()
	r.POST("/people/:id/refresh", refreshPersonHandler(svc, false))
	r.POST("/people/:id/translate", refreshPersonHandler(svc, true))
	r.POST("/media/:id/people/refresh", refreshMediaPeopleHandler(svc))
	r.POST("/media/:id/people/translate", mediaPeopleHandler(svc, true))
	r.POST("/media/:id/douban", enrichMediaDoubanHandler(svc))
	r.PUT("/settings", peopleTranslationSettingsHandler(svc))
	for _, path := range []string{"/people/a/refresh", "/people/a/translate", "/media/a/people/refresh", "/media/a/people/translate", "/media/a/douban", "/settings"} {
		method := "POST"
		if path == "/settings" {
			method = "PUT"
		}
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(method, path, nil))
		if response.Code != 503 || !strings.Contains(response.Body.String(), "startup_not_ready") || response.Header().Get("Retry-After") != "3" {
			t.Fatalf("%s %d %s", path, response.Code, response.Body.String())
		}
	}
}

func TestManualPersonFailureAppearsInTaskTracker(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := &service.Container{Tasks: service.NewTaskTrackerService(zap.NewNop(), nil)}
	r := gin.New()
	r.POST("/people/:id/refresh", refreshPersonHandler(svc, false))
	response := httptest.NewRecorder()
	r.ServeHTTP(response, httptest.NewRequest("POST", "/people/invalid/refresh", nil))
	if response.Code != 404 {
		t.Fatalf("status = %d", response.Code)
	}
	snap := svc.Tasks.Snapshot()
	if len(snap.Active) != 0 || len(snap.Recent) != 1 || snap.Recent[0].Kind != service.TaskKindPeople || snap.Recent[0].Status != service.TaskStatusFailed {
		t.Fatalf("tasks = %#v", snap)
	}
}
