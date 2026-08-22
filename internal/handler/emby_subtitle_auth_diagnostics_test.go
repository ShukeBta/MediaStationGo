package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestEmbyExternalSubtitleDeliveryCredentialShapes(t *testing.T) {
	out := map[string]any{
		"MediaSources": []map[string]any{{
			"MediaStreams": []map[string]any{
				{"Type": "Subtitle", "IsExternal": true, "DeliveryUrl": "/emby/Videos/a/b/Subtitles/2/Stream.ass?api_key=header.payload.signature"},
				{"Type": "Subtitle", "IsExternal": true, "DeliveryUrl": "/emby/Videos/a/b/Subtitles/3/Stream.ass"},
			},
		}},
	}
	got := embyExternalSubtitleDeliveryCredentialShapes(out)
	if strings.Join(got, ",") != "jwt,missing" {
		t.Fatalf("credential shapes = %v", got)
	}
}

func TestEmbySubtitleRequestAuthDiagnosticRedactsCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "test-secret"
	token := signedTestToken(t, secret)
	core, observed := observer.New(zap.InfoLevel)
	router := gin.New()
	router.GET(
		"/emby/Videos/:id/:seg/Subtitles/:stream/Stream.:format",
		embyAuthRequiredWithSessionFallback(secret, zap.New(core)),
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)
	req := httptest.NewRequest(http.MethodGet, "/emby/Videos/media/media/Subtitles/2/Stream.ass?api_key="+token, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	entries := observed.FilterMessage("emby subtitle request auth diagnostic").All()
	if len(entries) != 1 {
		t.Fatalf("diagnostic entries = %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["query_api_key_shape"] != "jwt" || fields["incoming_token_shape"] != "jwt" {
		t.Fatalf("unexpected diagnostic fields: %#v", fields)
	}
	if strings.Contains(fmt.Sprint(fields), token) {
		t.Fatal("diagnostic log leaked credential")
	}
}
