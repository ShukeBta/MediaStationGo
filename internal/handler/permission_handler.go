// Package handler — 权限相关 HTTP Handler。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// PermissionHandler 权限相关 HTTP 处理。
type PermissionHandler struct {
	svc *service.Container
	log *zap.Logger
}

// NewPermissionHandler 创建权限处理器。
func NewPermissionHandler(svc *service.Container, log *zap.Logger) *PermissionHandler {
	return &PermissionHandler{svc: svc, log: log}
}

// GetUserPermissions 获取指定用户的权限（管理员）。
// GET /api/users/:id/permissions
func (h *PermissionHandler) GetUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user id required", "data": nil})
		return
	}

	// 权限检查：需要管理员权限
	role := middleware.GetUserRole(c)
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40301, "message": "admin only", "data": nil})
		return
	}

	perms, err := h.svc.Permissions.Effective(c.Request.Context(), userID)
	if err != nil {
		h.log.Error("get user permissions failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "internal error", "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "data": perms, "message": "ok"})
}

// UpdateUserPermissions 更新指定用户的权限（管理员）。
// PUT /api/users/:id/permissions
func (h *PermissionHandler) UpdateUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user id required", "data": nil})
		return
	}

	// 权限检查：需要管理员权限
	role := middleware.GetUserRole(c)
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40301, "message": "admin only", "data": nil})
		return
	}

	var req model.UserPermission
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "invalid request", "data": nil})
		return
	}

	if err := h.svc.Permissions.Save(c.Request.Context(), userID, &req); err != nil {
		h.log.Error("update user permissions failed", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "internal error", "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "permissions updated", "data": nil})
}

// ResetUserPermissions 重置指定用户的权限为默认值（管理员）。
// POST /api/users/:id/permissions/reset
func (h *PermissionHandler) ResetUserPermissions(c *gin.Context) {
	userID := c.Param("id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 40001, "message": "user id required", "data": nil})
		return
	}

	// 权限检查：需要管理员权限
	role := middleware.GetUserRole(c)
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"code": 40301, "message": "admin only", "data": nil})
		return
	}

	if _, err := h.svc.Permissions.Reset(c.Request.Context(), userID); err != nil {
		h.log.Error("reset user permissions failed", zap.Error(err), zap.String("user_id", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "internal error", "data": nil})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "permissions reset to default", "data": nil})
}

// GetMyPermissions 获取当前用户的权限。
// GET /api/auth/permissions
func (h *PermissionHandler) GetMyPermissions(c *gin.Context) {
	currentUserID := middleware.GetUserID(c)
	if currentUserID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "authentication required", "data": nil})
		return
	}

	perms, err := h.svc.Permissions.Effective(c.Request.Context(), currentUserID)
	if err != nil {
		if service.IsTransientDatabaseLock(err) {
			role := middleware.GetUserRole(c)
			tier := middleware.GetUserTier(c)
			c.JSON(http.StatusOK, gin.H{
				"code":    0,
				"message": "ok",
				"data": gin.H{
					"permissions": service.FallbackPermissions(currentUserID, role),
					"role":        role,
					"tier":        tier,
					"is_super":    role == "admin",
				},
			})
			return
		}
		h.log.Error("get my permissions failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": 50001, "message": "internal error", "data": nil})
		return
	}

	// 同时返回角色和等级信息
	role := middleware.GetUserRole(c)
	tier := middleware.GetUserTier(c)

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"permissions": perms,
			"role":        role,
			"tier":        tier,
			"is_super":    role == "admin",
		},
	})
}
