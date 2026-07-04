package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func TestEmbySessionsReturnsRealtimeSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracker := service.NewSessionTrackerService(zap.NewNop())
	tracker.RecordPlayback(t.Context(), "user-1", "viewer", "dev-1", "Apple TV", "Yamby", "10.0.0.8", "media-1", 1000, 2000, false)
	svc := &service.Container{Sessions: tracker}
	router := gin.New()
	router.GET("/Sessions", embySessionsHandler(svc))

	req := httptest.NewRequest(http.MethodGet, "/Sessions", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control = %q, want no-store", got)
	}
	var rows []map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("sessions = %d, want 1: %s", len(rows), w.Body.String())
	}
	if rows[0]["UserId"] != "user-1" || rows[0]["DeviceId"] != "dev-1" || rows[0]["Client"] != "Yamby" {
		t.Fatalf("session payload = %#v", rows[0])
	}
	if _, err := time.Parse(time.RFC3339Nano, rows[0]["LastActivityDate"].(string)); err != nil {
		t.Fatalf("LastActivityDate should be RFC3339 time, got %#v", rows[0]["LastActivityDate"])
	}
}
