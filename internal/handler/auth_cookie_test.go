package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
)

func TestSetAccessTokenCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "https://media.local/api/auth/login", nil)

	setAccessTokenCookie(c, "access-token", 3600)

	cookie := findResponseCookie(t, w, middleware.AccessTokenCookieName)
	if cookie.Value != "access-token" {
		t.Fatalf("cookie value = %q", cookie.Value)
	}
	if cookie.Path != middleware.AccessTokenCookiePath {
		t.Fatalf("cookie path = %q, want %q", cookie.Path, middleware.AccessTokenCookiePath)
	}
	if cookie.MaxAge != 3600 {
		t.Fatalf("cookie max age = %d, want 3600", cookie.MaxAge)
	}
	if !cookie.HttpOnly {
		t.Fatal("cookie should be HttpOnly")
	}
	if !cookie.Secure {
		t.Fatal("https request should set Secure cookie")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite = %v, want Lax", cookie.SameSite)
	}
}

func TestClearAccessTokenCookie(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/me/logout", nil)

	clearAccessTokenCookie(c)

	cookie := findResponseCookie(t, w, middleware.AccessTokenCookieName)
	if cookie.MaxAge >= 0 {
		t.Fatalf("clear cookie max age = %d, want negative", cookie.MaxAge)
	}
	if cookie.Path != middleware.AccessTokenCookiePath {
		t.Fatalf("cookie path = %q, want %q", cookie.Path, middleware.AccessTokenCookiePath)
	}
	if cookie.Secure {
		t.Fatal("plain http request should not set Secure cookie")
	}
}

func findResponseCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing response cookie %q", name)
	return nil
}
