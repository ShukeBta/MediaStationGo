package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func setAccessTokenCookie(c *gin.Context, token string, maxAgeSeconds int) {
	token = strings.TrimSpace(token)
	if token == "" {
		return
	}
	if maxAgeSeconds <= 0 {
		maxAgeSeconds = int(service.AccessTokenDuration.Seconds())
	}
	writeAccessTokenCookie(c, token, maxAgeSeconds)
}

func clearAccessTokenCookie(c *gin.Context) {
	writeAccessTokenCookie(c, "", -1)
}

func writeAccessTokenCookie(c *gin.Context, value string, maxAgeSeconds int) {
	cookie := &http.Cookie{
		Name:     middleware.AccessTokenCookieName,
		Value:    value,
		Path:     middleware.AccessTokenCookiePath,
		MaxAge:   maxAgeSeconds,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(c),
	}
	if maxAgeSeconds > 0 {
		cookie.Expires = time.Now().Add(time.Duration(maxAgeSeconds) * time.Second)
	} else if maxAgeSeconds < 0 {
		cookie.Expires = time.Unix(0, 0)
	}
	http.SetCookie(c.Writer, cookie)
}

func requestIsHTTPS(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	if c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}
