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
	req.Header.Set("X-Emby-Device-Id", "test-device")
	req.Header.Set("User-Agent", "test-client")
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
	if got := fmt.Sprint(fields["compat_session_key_kinds"]); got != "[device ua]" {
		t.Fatalf("compat session key kinds = %q", got)
	}
	if strings.Contains(fmt.Sprint(fields), token) {
		t.Fatal("diagnostic log leaked credential")
	}
}

func TestEmbyPlaybackInfoSeedsCompatibilitySessionForHeaderlessSubtitle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "test-secret"
	token := signedTestToken(t, secret)
	userAgent := "filmly-tv-test-compat-session"
	remoteAddr := "203.0.113.17:12345"
	core, observed := observer.New(zap.InfoLevel)
	router := gin.New()
	auth := embyAuthRequiredWithSessionFallback(secret, zap.New(core))
	router.POST("/emby/Items/:id/PlaybackInfo", auth, func(c *gin.Context) { c.Status(http.StatusNoContent) })
	router.GET(
		"/emby/Videos/:id/:seg/Subtitles/:stream/Stream.:format",
		auth,
		func(c *gin.Context) { c.Status(http.StatusNoContent) },
	)

	playbackReq := httptest.NewRequest(http.MethodPost, "/emby/Items/media/PlaybackInfo", nil)
	playbackReq.RemoteAddr = remoteAddr
	playbackReq.Header.Set("X-Emby-Token", token)
	playbackReq.Header.Set("User-Agent", userAgent)
	playbackRes := httptest.NewRecorder()
	router.ServeHTTP(playbackRes, playbackReq)
	if playbackRes.Code != http.StatusNoContent {
		t.Fatalf("PlaybackInfo status = %d", playbackRes.Code)
	}

	subtitleReq := httptest.NewRequest(http.MethodGet, "/emby/Videos/media/media/Subtitles/2/Stream.ass", nil)
	subtitleReq.RemoteAddr = remoteAddr
	subtitleReq.Header.Set("User-Agent", userAgent)
	subtitleRes := httptest.NewRecorder()
	router.ServeHTTP(subtitleRes, subtitleReq)
	if subtitleRes.Code != http.StatusNoContent {
		t.Fatalf("headerless subtitle status = %d", subtitleRes.Code)
	}
	entries := observed.FilterMessage("emby subtitle request auth diagnostic").All()
	if len(entries) != 1 {
		t.Fatalf("subtitle diagnostic entries = %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["compat_session_fallback_used"] != true || fmt.Sprint(fields["compat_session_key_kinds"]) != "[ua]" {
		t.Fatalf("unexpected fallback diagnostic: %#v", fields)
	}
}
