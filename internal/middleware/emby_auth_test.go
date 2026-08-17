package middleware

import (
	"encoding/json"
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
		{name: "x emby authorization", headerKey: "X-Emby-Authorization", headerVal: `MediaBrowser Client="VidHub", Token="` + token + `"`},
		{name: "x mediabrowser authorization", headerKey: "X-MediaBrowser-Authorization", headerVal: `MediaBrowser Client="Emby Theater", Token="` + token + `"`},
		{name: "query api key", query: "?api_key=" + token},
		{name: "query x emby token", query: "?X-Emby-Token=" + token},
		{name: "query x mediabrowser token", query: "?X-MediaBrowser-Token=" + token},
		{name: "query token", query: "?token=" + token},
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

// TestEmbyAuthRequiredRejectsMediaBrowserHeaderWithoutToken guards against a
// subtle parsing bug: some clients (e.g. Hills) send an Authorization header
// like `MediaBrowser Client="Hills", DeviceId="..."` that contains no Token=.
// The parser must NOT treat the whole header value as the token — otherwise it
// is fed to JWT parsing and fails with 40101 "Invalid token" on every request
// after a successful login.
func TestEmbyAuthRequiredRejectsMediaBrowserHeaderWithoutToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "test-secret"
	token := signedTestToken(t, secret)

	// 1) A MediaBrowser header with a Token= must still work.
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Header("X-Emby-Token", token)
		c.Next()
	})
	router.GET("/with/header/token", EmbyAuthRequired(secret), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/with/header/token", nil)
	req.Header.Set("Authorization", `MediaBrowser Client="Hills", Token="`+token+`"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with Token=, got %d: %s", w.Code, w.Body.String())
	}

	// 2) A MediaBrowser header WITHOUT Token= must be treated as "no token",
	//    i.e. the request must fail with 40101 (Unauthorized), NOT "Invalid token".
	req = httptest.NewRequest(http.MethodGet, "/with/header/token", nil)
	req.Header.Set("Authorization", `MediaBrowser Client="Hills", DeviceId="device-42"`)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for no-token MediaBrowser header, got %d", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if msg, _ := body["Message"].(string); msg != "Unauthorized" {
		t.Fatalf("expected Message=Unauthorized for no-token header, got %q", msg)
	}
}

// TestEmbyAuthRequiredExtractsTokenFromEmbyHeaderWithUserId covers the real
// RodelPlayer / 小幻影视 request shape: all credentials live in a single
// X-Emby-Authorization header that carries BOTH UserId and Token:
//
//	Emby UserId="<uuid>", Client="RodelPlayer", Device="WHILETRUE",
//	DeviceId="...", Version="2.2607.7.0", Token="<jwt>"
//
// The parser must extract the pure JWT from Token="...", NOT the whole header
// value — otherwise JWT parsing fails with 40101 "Invalid token".
func TestEmbyAuthRequiredExtractsTokenFromEmbyHeaderWithUserId(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "test-secret"
	token := signedTestToken(t, secret)

	// 1) Full RodelPlayer-style header with both UserId and Token must pass.
	router := gin.New()
	router.GET("/with/header/token", EmbyAuthRequired(secret), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	req := httptest.NewRequest(http.MethodGet, "/with/header/token", nil)
	req.Header.Set("X-Emby-Authorization",
		`Emby UserId="23d5bec4-9ffc-4178-be9b-f5496f8b1b54", Client="RodelPlayer", Device="WHILETRUE", DeviceId="7aacf5c9-815a-4e9c-8b26-6a842da9fbd4", Version="2.2607.7.0", Token="`+token+`"`)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with Emby UserId+Token header, got %d: %s", w.Code, w.Body.String())
	}

	// 2) The same header WITHOUT Token= (UserId only) must NOT be treated as a
	//    token — it should fail with 40101 "Unauthorized" (no token), not
	//    "Invalid token" from misparsed JWT.
	req = httptest.NewRequest(http.MethodGet, "/with/header/token", nil)
	req.Header.Set("X-Emby-Authorization",
		`Emby UserId="23d5bec4-9ffc-4178-be9b-f5496f8b1b54", Client="RodelPlayer", Device="WHILETRUE", DeviceId="7aacf5c9-815a-4e9c-8b26-6a842da9fbd4"`)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for Emby UserId-only header, got %d", w.Code)
	}
}
