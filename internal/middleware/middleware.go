// Package middleware 暴露 Gin 中间件，用于 HTTP 服务器：
// 请求日志、CORS、JWT 认证、管理员守卫和权限检查。
package middleware

import (
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

// Context keys for values produced by the auth middleware.
const (
	CtxUserID   = "ctx_user_id"
	CtxUserRole = "ctx_user_role"
	CtxUserTier = "ctx_user_tier"
)

// RequestLogger logs one structured line per request.
func RequestLogger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Info("http",
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("dur", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		)
	}
}

// CORS implements a cross-origin policy. When debug is true and origins is
// empty, all origins are allowed (dev convenience). In production
// (debug=false) with an empty origins list, CORS headers are omitted
// entirely so the browser enforces same-origin by default.
func CORS(origins []string, debug bool) gin.HandlerFunc {
	allowAll := len(origins) == 0 && debug
	allowed := make(map[string]struct{}, len(origins))
	for _, o := range origins {
		allowed[strings.TrimSpace(o)] = struct{}{}
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

// RateLimiter is a simple per-IP sliding-window rate limiter that runs
// entirely in-process. It is intended for auth endpoints (login, register)
// where brute-force protection is critical.
type RateLimiter struct {
	mu       sync.Mutex
	window   time.Duration
	max      int
	requests map[string][]time.Time
}

// NewRateLimiter creates a rate limiter allowing max requests per window
// per client IP.
func NewRateLimiter(max int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		window:   window,
		max:      max,
		requests: make(map[string][]time.Time),
	}
	go rl.cleanup()
	return rl
}

func (rl *RateLimiter) cleanup() {
	for {
		time.Sleep(5 * time.Minute)
		rl.mu.Lock()
		now := time.Now()
		for ip, times := range rl.requests {
			var valid []time.Time
			for _, t := range times {
				if now.Sub(t) <= rl.window {
					valid = append(valid, t)
				}
			}
			if len(valid) == 0 {
				delete(rl.requests, ip)
			} else {
				rl.requests[ip] = valid
			}
		}
		rl.mu.Unlock()
	}
}

// Allow returns true if the request from ip is within the rate limit.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	times := rl.requests[ip]
	var valid []time.Time
	for _, t := range times {
		if now.Sub(t) <= rl.window {
			valid = append(valid, t)
		}
	}
	if len(valid) >= rl.max {
		rl.requests[ip] = valid
		return false
	}
	rl.requests[ip] = append(valid, now)
	return true
}

// RateLimit returns a Gin middleware that rejects requests exceeding the
// per-IP rate limit with 429 Too Many Requests.
func RateLimit(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !limiter.Allow(c.ClientIP()) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"code":    42901,
				"message": "too many requests, please try again later",
			})
			return
		}
		c.Next()
	}
}

// Claims is the JWT payload we issue.
type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role"`
	Tier   string `json:"tier,omitempty"`
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
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxUserRole, claims.Role)
		c.Set(CtxUserTier, claims.Tier)
		c.Next()
	}
}

// AdminRequired must run AFTER AuthRequired; it enforces role == "admin".
func AdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get(CtxUserRole)
		if role != "admin" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40301, "message": "admin only"})
			return
		}
		c.Next()
	}
}

// PlusOrAdminRequired enforces role == "admin" or tier == "plus".
func PlusOrAdminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get(CtxUserRole)
		tier, _ := c.Get(CtxUserTier)
		if role != "admin" && tier != "plus" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40301, "message": "plus or admin only"})
			return
		}
		c.Next()
	}
}

// GetUserID extracts the user ID from the Gin context.
func GetUserID(c *gin.Context) string {
	if uid, exists := c.Get(CtxUserID); exists {
		return uid.(string)
	}
	return ""
}

// GetUserRole extracts the user role from the Gin context.
func GetUserRole(c *gin.Context) string {
	if role, exists := c.Get(CtxUserRole); exists {
		return role.(string)
	}
	return ""
}

// GetUserTier extracts the user tier from the Gin context.
func GetUserTier(c *gin.Context) string {
	if tier, exists := c.Get(CtxUserTier); exists {
		return tier.(string)
	}
	return ""
}

// IsAdmin checks if the current user is an admin.
func IsAdmin(c *gin.Context) bool {
	return GetUserRole(c) == "admin"
}

// IsPlus checks if the current user is a plus subscriber.
func IsPlus(c *gin.Context) bool {
	return GetUserTier(c) == "plus" || GetUserRole(c) == "admin"
}

// IsSuperUser checks if the current user is a super user (admin or plus).
func IsSuperUser(c *gin.Context) bool {
	return IsAdmin(c) || IsPlus(c)
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
	return ""
}
