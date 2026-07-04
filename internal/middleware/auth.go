package middleware

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Claims is the JWT payload we issue.
type Claims struct {
	UserID  string `json:"uid"`
	Role    string `json:"role"`
	Tier    string `json:"tier,omitempty"`
	Purpose string `json:"purpose,omitempty"`
	MediaID string `json:"media_id,omitempty"`
	jwt.RegisteredClaims
}

// AuthRequired parses and validates a JWT from the Authorization header
// (Bearer ...) or the `token` query parameter (used by <video>.src).
func AuthRequired(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := extractToken(c)
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "missing token"})
			return
		}
		claims := &Claims{}
		_, err := jwt.ParseWithClaims(raw, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(secret), nil
		})
		if err != nil || claims.UserID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "invalid token"})
			return
		}
		if !scopedTokenAllowedForRequest(c, claims) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40304, "message": "token scope denied"})
			return
		}
		syncAccessTokenCookie(c, raw, claims)
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUserRole, claims.Role)
		c.Set(CtxUserTier, claims.Tier)
		c.Set(CtxTokenPurpose, claims.Purpose)
		c.Set(CtxTokenMediaID, claims.MediaID)
		c.Next()
	}
}

func syncAccessTokenCookie(c *gin.Context, raw string, claims *Claims) {
	if c == nil || claims == nil || strings.TrimSpace(raw) == "" || strings.TrimSpace(claims.Purpose) != "" {
		return
	}
	if existing, err := c.Cookie(AccessTokenCookieName); err == nil && existing == raw {
		return
	}
	maxAge := int(time.Hour.Seconds())
	expires := time.Now().Add(time.Hour)
	if claims.ExpiresAt != nil {
		expires = claims.ExpiresAt.Time
		ttl := time.Until(expires)
		if ttl <= 0 {
			return
		}
		maxAge = int(ttl.Seconds())
		if maxAge < 1 {
			maxAge = 1
		}
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    raw,
		Path:     AccessTokenCookiePath,
		MaxAge:   maxAge,
		Expires:  expires,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(c),
	})
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

func scopedTokenAllowedForRequest(c *gin.Context, claims *Claims) bool {
	if claims == nil || strings.TrimSpace(claims.Purpose) == "" {
		return true
	}
	switch strings.TrimSpace(claims.Purpose) {
	case "external_play":
		return externalPlaybackTokenAllowedPath(c, strings.TrimSpace(claims.MediaID))
	default:
		return false
	}
}

func externalPlaybackTokenAllowedPath(c *gin.Context, mediaID string) bool {
	if c == nil || c.Request == nil || strings.TrimSpace(mediaID) == "" {
		return false
	}
	pathValue := strings.Trim(c.Request.URL.Path, "/")
	segments := strings.Split(pathValue, "/")
	for i := range segments {
		segments[i] = strings.TrimSpace(segments[i])
	}
	if len(segments) >= 3 && strings.EqualFold(segments[0], "api") {
		switch strings.ToLower(segments[1]) {
		case "stream", "hls":
			return segments[2] == mediaID
		case "cloud":
			return len(segments) >= 3 &&
				strings.EqualFold(segments[2], "play") &&
				strings.TrimSpace(c.Query("media_id")) == mediaID
		}
	}
	return false
}

func extractToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	for _, header := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
			return value
		}
	}
	for _, key := range []string{"token", "api_key", "apiKey", "ApiKey"} {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
	}
	if cookie, err := c.Cookie(AccessTokenCookieName); err == nil {
		return strings.TrimSpace(cookie)
	}
	return ""
}
