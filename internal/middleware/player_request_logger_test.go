package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/gin-gonic/gin"
)

func TestPlayerRequestLogExcludesCredentialsAndUsesRouteTemplate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var log model.PlayerRequestLog
	r.Use(PlayerRequestLogger(nil, func(row model.PlayerRequestLog) { log = row }))
	r.POST("/emby/Items/:id/PlaybackInfo", func(c *gin.Context) { c.JSON(200, gin.H{"AccessToken": "response-secret"}) })
	req := httptest.NewRequest(http.MethodPost, "/emby/Items/path-secret/PlaybackInfo?api_key=query-secret&passkey=passkey-secret&MediaSourceId=source-1&ItemId=https%3A%2F%2Fhost%3Ftoken%3Dnested-secret", strings.NewReader(`{"Password":"body-secret"}`))
	req.Header.Set("Authorization", "Bearer header-secret")
	req.Header.Set("Cookie", "session=cookie-secret")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	encoded, _ := json.Marshal(log)
	if strings.Contains(string(encoded), "secret") {
		t.Fatalf("credentials in log: %s", encoded)
	}
	if log.Route != "/emby/Items/:id/PlaybackInfo" || log.Status != 200 || !strings.Contains(log.Query, "source-1") || !strings.Contains(log.Query, "redacted") {
		t.Fatalf("unexpected log: %#v", log)
	}
}
