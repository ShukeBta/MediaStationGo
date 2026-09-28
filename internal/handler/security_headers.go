package handler

import "github.com/gin-gonic/gin"

// securityHeaders adds browser-side hardening that is safe for the web UI and
// native Emby clients. HSTS is intentionally left to the HTTPS reverse proxy,
// because the application also serves plain HTTP on trusted local networks.
func securityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "SAMEORIGIN")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		c.Next()
	}
}
