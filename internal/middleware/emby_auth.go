// Package middleware — Emby API 兼容层认证中间件。
// 支持 X-Emby-Token / X-MediaBrowser-Token / Bearer / MediaBrowser / URL token。
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// EmbyCtxUserID 是 Emby 认证中间件设置的用户 ID 上下文键。
const EmbyCtxUserID = "emby_user_id"

// EmbyAuthRequired Emby 认证中间件。
// 按优先级尝试以下认证方式：
// 1. X-Emby-Token / X-MediaBrowser-Token 请求头
// 2. Authorization: Bearer <token> / MediaBrowser Token="<token>" 请求头
// 3. X-Emby-Authorization / X-MediaBrowser-Authorization: MediaBrowser Token="<token>"
// 4. ?token= / ?api_key= / ?apiKey= / ?X-Emby-Token= URL 参数
func EmbyAuthRequired(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractEmbyToken(c)

		if token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"Code":    40101,
				"Message": "Unauthorized",
			})
			c.Abort()
			return
		}

		// 解析 JWT
		claims := &Claims{}
		parsed, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(secret), nil
		})

		if err != nil || !parsed.Valid || claims.UserID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"Code":    40101,
				"Message": "Invalid token",
			})
			c.Abort()
			return
		}

		c.Set(EmbyCtxUserID, claims.UserID)
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUserRole, claims.Role)
		c.Set(CtxUserTier, claims.Tier)
		c.Next()
	}
}

func extractEmbyToken(c *gin.Context) string {
	for _, header := range []string{"X-Emby-Token", "X-MediaBrowser-Token"} {
		if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
			return value
		}
	}

	for _, header := range []string{"Authorization", "X-Emby-Authorization", "X-MediaBrowser-Authorization"} {
		if value := strings.TrimSpace(c.GetHeader(header)); value != "" {
			if token := tokenFromAuthHeader(value); token != "" {
				return token
			}
		}
	}

	for _, key := range []string{"token", "api_key", "apiKey", "ApiKey", "X-Emby-Token", "X-MediaBrowser-Token"} {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
	}
	return ""
}

func tokenFromAuthHeader(value string) string {
	if strings.HasPrefix(value, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(value, "Bearer "))
	}
	if strings.HasPrefix(value, "MediaBrowser ") || strings.HasPrefix(value, "Emby ") || strings.Contains(value, "Token=") {
		if token := tokenFromMediaBrowserAuth(value); token != "" {
			return token
		}
		if strings.HasPrefix(value, "Emby ") {
			return strings.TrimSpace(strings.TrimPrefix(value, "Emby "))
		}
	}
	return value
}

func tokenFromMediaBrowserAuth(value string) string {
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		for _, prefix := range []string{"MediaBrowser ", "Emby "} {
			part = strings.TrimSpace(strings.TrimPrefix(part, prefix))
		}
		if !strings.HasPrefix(part, "Token=") {
			continue
		}
		token := strings.TrimSpace(strings.TrimPrefix(part, "Token="))
		return strings.Trim(token, `"`)
	}
	return ""
}

// GetEmbyUserID 从上下文中获取 Emby 用户 ID。
func GetEmbyUserID(c *gin.Context) string {
	if uid, exists := c.Get(EmbyCtxUserID); exists {
		return uid.(string)
	}
	return ""
}
