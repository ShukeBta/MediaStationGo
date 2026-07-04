// Package handler — 令牌刷新 HTTP Handler。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// RefreshHandler 令牌刷新 HTTP 处理。
type RefreshHandler struct {
	svc *service.Container
	log *zap.Logger
}

// NewRefreshHandler 创建刷新令牌处理器。
func NewRefreshHandler(svc *service.Container, log *zap.Logger) *RefreshHandler {
	return &RefreshHandler{svc: svc, log: log}
}

// RefreshTokenRequest 刷新令牌请求结构。
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RefreshToken 刷新访问令牌。
// POST /api/auth/refresh
func (h *RefreshHandler) RefreshToken(c *gin.Context) {
	var req RefreshTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "refresh_token required", "data": nil})
		return
	}

	tokens, err := h.svc.Auth.RefreshTokens(c.Request.Context(), req.RefreshToken)
	if err != nil {
		h.log.Debug("token refresh failed", zap.Error(err))

		// 根据错误类型返回不同状态码
		switch err {
		case service.ErrInvalidRefreshToken:
			c.JSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "invalid refresh token", "data": nil})
		case service.ErrTokenExpired:
			c.JSON(http.StatusUnauthorized, gin.H{"code": 40102, "message": "refresh token expired", "data": nil})
		case service.ErrTokenRevoked:
			c.JSON(http.StatusUnauthorized, gin.H{"code": 40103, "message": "refresh token revoked", "data": nil})
		case service.ErrUserInactive:
			c.JSON(http.StatusForbidden, gin.H{"code": 40302, "message": "user account is disabled", "data": nil})
		case service.ErrUserExpired:
			c.JSON(http.StatusForbidden, gin.H{"code": 40303, "message": "user account has expired", "data": nil})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "internal error", "data": nil})
		}
		return
	}

	setAccessTokenCookie(c, tokens.AccessToken, int(tokens.ExpiresIn))
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"token":         tokens.AccessToken,
			"refresh_token": tokens.RefreshToken,
			"expires_in":    tokens.ExpiresIn,
			"token_type":    tokens.TokenType,
		},
	})
}

// Logout 登出当前用户。
// POST /api/auth/logout
func (h *RefreshHandler) Logout(c *gin.Context) {
	clearAccessTokenCookie(c)
	userID := c.GetString("ctx_user_id")
	if userID == "" {
		c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
		return
	}

	if err := h.svc.Auth.Logout(c.Request.Context(), userID); err != nil {
		h.log.Warn("logout failed", zap.Error(err), zap.String("user_id", userID))
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": nil})
}
