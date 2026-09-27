// Package middleware — 权限检查中间件。
// 注意：实际的权限检查在 handler 层通过 PermissionService 实现。
// 此中间件主要用于设置上下文和基本的角色/等级检查。
package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequirePermission 创建权限检查中间件标记。
// 实际的权限检查由 handler 中的 PermissionService 执行。
// 此中间件确保请求已经过身份验证。
func RequirePermission(permissionKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		role := GetUserRole(c)

		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "authentication required",
				"data":    nil,
			})
			return
		}

		// 只有管理员绕过用户级权限。
		if role == "admin" {
			c.Next()
			return
		}

		// 将权限键存储到上下文中供 handler 使用
		c.Set("permission_key", permissionKey)
		c.Next()
	}
}

// RequireAnyPermission 创建需要任意一个权限的中间件标记。
func RequireAnyPermission(permissionKeys ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		role := GetUserRole(c)

		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "authentication required",
				"data":    nil,
			})
			return
		}

		// 只有管理员绕过用户级权限。
		if role == "admin" {
			c.Next()
			return
		}

		// 将权限键数组存储到上下文中供 handler 使用
		c.Set("permission_keys", permissionKeys)
		c.Next()
	}
}

// RequireAllPermissions 创建需要所有权限的中间件标记。
func RequireAllPermissions(permissionKeys ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := GetUserID(c)
		role := GetUserRole(c)

		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"code":    40101,
				"message": "authentication required",
				"data":    nil,
			})
			return
		}

		// 只有管理员绕过用户级权限。
		if role == "admin" {
			c.Next()
			return
		}

		c.Set("permission_keys", permissionKeys)
		c.Next()
	}
}

// GetPermissionKey 从上下文中获取存储的权限键。
func GetPermissionKey(c *gin.Context) string {
	if key, exists := c.Get("permission_key"); exists {
		return key.(string)
	}
	return ""
}

// GetPermissionKeys 从上下文中获取存储的权限键数组。
func GetPermissionKeys(c *gin.Context) []string {
	if keys, exists := c.Get("permission_keys"); exists {
		return keys.([]string)
	}
	return nil
}
