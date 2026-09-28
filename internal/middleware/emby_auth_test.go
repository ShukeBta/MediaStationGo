package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestEmbyAuthRequiredAcceptsEmbyClientTokenFormats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "test-secret"
	token := signedTestToken(t, secret)

	tests := []struct {
		name      string
		headerKey string
		headerVal string
		query     string
	}{
		{name: "x emby token", headerKey: "X-Emby-Token", headerVal: token},
		{name: "x mediabrowser token", headerKey: "X-MediaBrowser-Token", headerVal: token},
		{name: "authorization mediabrowser token", headerKey: "Authorization", headerVal: `MediaBrowser Client="Infuse", Token="` + token + `"`},
		{name: "authorization emby client token", headerKey: "Authorization", headerVal: `Emby Client="Moonfin", Device="Android TV", DeviceId="moonfin-tv", Version="1.0.0", Token="` + token + `"`},
		{name: "authorization emby bare token", headerKey: "Authorization", headerVal: "Emby " + token},
		{name: "x emby authorization", headerKey: "X-Emby-Authorization", headerVal: `MediaBrowser Client="VidHub", Token="` + token + `"`},
		{name: "x mediabrowser authorization", headerKey: "X-MediaBrowser-Authorization", headerVal: `MediaBrowser Client="Emby Theater", Token="` + token + `"`},
		{name: "query api key", query: "?api_key=" + token},
		{name: "query x emby token", query: "?X-Emby-Token=" + token},
		{name: "query x mediabrowser token", query: "?X-MediaBrowser-Token=" + token},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.GET("/Users/Me", EmbyAuthRequired(secret), func(c *gin.Context) {
				c.String(http.StatusOK, GetUserID(c))
			})
			req := httptest.NewRequest(http.MethodGet, "/Users/Me"+tt.query, nil)
			if tt.headerKey != "" {
				req.Header.Set(tt.headerKey, tt.headerVal)
			}
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}
			if got := w.Body.String(); got != "user-1" {
				t.Fatalf("expected user id, got %q", got)
			}
		})
	}
}

func signedTestToken(t *testing.T, secret string) string {
	t.Helper()
	raw := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{
		UserID: "user-1",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	token, err := raw.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return token
}

func TestEmbyAuthRejectsScopedPlaybackTokens(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "scope-test-secret"
	for _, purpose := range []string{"external_play", "unknown_scope"} {
		t.Run(purpose, func(t *testing.T) {
			token := signedMiddlewareTestToken(t, secret, Claims{
				UserID: "user-1", Role: "admin", Purpose: purpose, MediaID: "media-1",
				RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
			})
			for _, path := range []string{"/Users/Me", "/emby/Items", "/emby/Videos/media-1/stream", "/videos/media-2/stream", "/emby/Sessions/Playing"} {
				router := gin.New()
				router.Any(path, EmbyAuthRequired(secret), func(c *gin.Context) {
					c.Status(http.StatusNoContent)
				})
				req := httptest.NewRequest(http.MethodGet, path+"?api_key="+token, nil)
				w := httptest.NewRecorder()
				router.ServeHTTP(w, req)
				if w.Code != http.StatusForbidden {
					t.Fatalf("%s: scoped token reached Emby handler, status=%d", path, w.Code)
				}
			}
		})
	}
}

func TestExternalPlaybackTokenRemainsLimitedToItsWebMediaRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "scope-test-secret"
	token := signedMiddlewareTestToken(t, secret, Claims{
		UserID: "user-1", Role: "user", Purpose: "external_play", MediaID: "media-1",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	})
	router := gin.New()
	router.GET("/api/stream/:id", AuthRequired(secret), func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		id     string
		status int
	}{{"media-1", http.StatusNoContent}, {"media-2", http.StatusForbidden}} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/stream/"+tc.id+"?token="+token, nil))
		if w.Code != tc.status {
			t.Fatalf("%s: status=%d, want %d", tc.id, w.Code, tc.status)
		}
	}
}
