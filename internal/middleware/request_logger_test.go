package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLoggerRecordsMediaSourceIDWithoutQueryCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zap.InfoLevel)
	router := gin.New()
	router.Use(RequestLogger(zap.New(core)))
	router.GET("/emby/Items/:id/PlaybackInfo", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/emby/Items/item-1/PlaybackInfo?MediaSourceId=source-2&api_key=secret-token",
		nil,
	)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["path"] != "/emby/Items/item-1/PlaybackInfo" || fields["media_source_id"] != "source-2" {
		t.Fatalf("log fields = %#v", fields)
	}
	if _, ok := fields["query"]; ok {
		t.Fatalf("request logger must not log credential-bearing query: %#v", fields)
	}
}
