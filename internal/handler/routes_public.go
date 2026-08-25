// Package handler — public authentication routes.
package handler

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerPublicAuthRoutes(api *gin.RouterGroup, svc *service.Container, log *zap.Logger) {
	// Rate limiter for credential endpoints (login/register): brute-force
	// protection. 30/min per IP tolerates many users behind a single NAT
	// or reverse-proxy IP while still throttling password guessing.
	authLimiter := middleware.NewRateLimiter(30, 1*time.Minute)

	// Public auth. Account creation is intentionally not public: users must be
	// created by an administrator (or through the separately governed Bot
	// registration flow), then authenticate through this login endpoint.
	auth := api.Group("/auth")
	{
		auth.POST("/login", middleware.RateLimit(authLimiter), loginHandler(svc))
		// /auth/refresh 用 RefreshHandler.RefreshToken：它从 body 读
		// refresh_token 并签发新 access/refresh 对。旧的 refreshHandler
		// 依赖 AuthRequired 中间件，永远 401，因此弃用。
		//
		// 刷新端点【不】做 IP 限流：刷新本身就是防止掉登录的机制，且已
		// 由一次性轮换的 refresh token 强校验。若按 IP 限流，多个用户/
		// 标签页共用一个反代 IP 时会把正常刷新打成 429，反而导致频繁
		// 掉登录。
		refreshHd := NewRefreshHandler(svc, log)
		auth.POST("/refresh", refreshHd.RefreshToken)
	}

}
