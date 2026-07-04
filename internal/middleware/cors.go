package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS implements a cross-origin policy. When debug is true and origins is
// empty, all origins are allowed (dev convenience). In production
// (debug=false) with an empty origins list, CORS headers are omitted
// entirely so the browser enforces same-origin by default.
func CORS(origins []string, debug bool) gin.HandlerFunc {
	allowAll := len(origins) == 0 && debug
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		origin := strings.TrimSpace(o)
		if origin == "*" {
			allowAll = true
			continue
		}
		if origin != "" {
			allowed[origin] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if allowAll {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if _, ok := allowed[origin]; ok && origin != "" {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Vary", "Origin")
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Requested-With, X-Emby-Token, X-MediaBrowser-Token, X-Emby-Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
