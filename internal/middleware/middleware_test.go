package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func TestCORSWildcardOriginAllowsProductionPreflight(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(CORS([]string{"*"}, false))
	router.GET("/emby/System/Info/Public", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodOptions, "/emby/System/Info/Public", nil)
	req.Header.Set("Origin", "http://senplayer.local")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want *", got)
	}
}

func TestAuthRequiredAcceptsAccessTokenCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "cookie-secret"
	token := signedMiddlewareTestToken(t, secret, Claims{
		UserID: "user-1",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	router := gin.New()
	router.Use(AuthRequired(secret))
	router.GET("/api/img", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetString(CtxUserID)})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/img?url=https%3A%2F%2Fexample.test%2Fposter.jpg", nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: token})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthRequiredKeepsExplicitQueryTokenPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "cookie-secret"
	accountToken := signedMiddlewareTestToken(t, secret, Claims{
		UserID: "user-1",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	scopedToken := signedMiddlewareTestToken(t, secret, Claims{
		UserID:  "user-1",
		Role:    "admin",
		Purpose: "external_play",
		MediaID: "media-1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	router := gin.New()
	router.Use(AuthRequired(secret))
	router.GET("/api/me", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/me?token="+scopedToken, nil)
	req.AddCookie(&http.Cookie{Name: AccessTokenCookieName, Value: accountToken})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
}

func TestAuthRequiredSyncsAccessTokenCookieFromBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "cookie-secret"
	token := signedMiddlewareTestToken(t, secret, Claims{
		UserID: "user-1",
		Role:   "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	router := gin.New()
	router.Use(AuthRequired(secret))
	router.GET("/api/discover/feed", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodGet, "/api/discover/feed", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	cookie := middlewareTestResponseCookie(t, w, AccessTokenCookieName)
	if cookie.Value != token {
		t.Fatal("synced cookie should contain the bearer token")
	}
	if cookie.Path != AccessTokenCookiePath {
		t.Fatalf("cookie path = %q, want %q", cookie.Path, AccessTokenCookiePath)
	}
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie flags not suitable: httpOnly=%v sameSite=%v", cookie.HttpOnly, cookie.SameSite)
	}
}

func TestAuthRequiredDoesNotSyncScopedPlaybackTokenCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	secret := "cookie-secret"
	token := signedMiddlewareTestToken(t, secret, Claims{
		UserID:  "user-1",
		Role:    "admin",
		Purpose: "external_play",
		MediaID: "media-1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})

	router := gin.New()
	router.Use(AuthRequired(secret))
	router.GET("/api/stream/media-1", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/stream/media-1?token="+token, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if cookie := optionalMiddlewareTestResponseCookie(w, AccessTokenCookieName); cookie != nil {
		t.Fatalf("scoped playback token should not be synced as web cookie: %#v", cookie)
	}
}

func signedMiddlewareTestToken(t *testing.T, secret string, claims Claims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func middlewareTestResponseCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	cookie := optionalMiddlewareTestResponseCookie(w, name)
	if cookie == nil {
		t.Fatalf("missing response cookie %q", name)
	}
	return cookie
}

func optionalMiddlewareTestResponseCookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
