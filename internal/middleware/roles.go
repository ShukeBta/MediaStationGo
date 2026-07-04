package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

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
