package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const embyCtxUserName = "emby_user_name"

func activeUserRequired(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		userID, _ := uid.(string)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "missing user"})
			return
		}
		u, err := svc.Repo.User.FindByID(c.Request.Context(), userID)
		if err != nil {
			if service.IsTransientDatabaseLock(err) {
				c.Header("Retry-After", "1")
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": 50301, "message": "user authorization temporarily unavailable"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "user not found"})
			return
		}
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": 40101, "message": "user not found"})
			return
		}
		if !u.IsActive {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40302, "message": "user account is disabled"})
			return
		}
		if u.ExpiredAt != nil && time.Now().After(*u.ExpiredAt) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": 40303, "message": "user account has expired"})
			return
		}
		// Signed claims establish identity, but mutable authorization must use
		// the current account so existing tokens cannot retain revoked access.
		c.Set(middleware.CtxUserRole, u.Role)
		c.Set(middleware.CtxUserTier, u.Tier)
		c.Next()
	}
}

func activeEmbyUserRequired(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		userID, _ := uid.(string)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Code": 40101, "Message": "User not found"})
			return
		}
		u, err := svc.Repo.User.FindByID(c.Request.Context(), userID)
		if err != nil {
			if service.IsTransientDatabaseLock(err) {
				c.Header("Retry-After", "1")
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"Code": 50301, "Message": "User authorization temporarily unavailable"})
				return
			}
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Code": 40101, "Message": "User not found"})
			return
		}
		if u == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Code": 40101, "Message": "User not found"})
			return
		}
		if !u.IsActive {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"Code": 40302, "Message": "User account is disabled"})
			return
		}
		if u.ExpiredAt != nil && time.Now().After(*u.ExpiredAt) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"Code": 40303, "Message": "User account has expired"})
			return
		}
		c.Set(embyCtxUserName, u.Username)
		c.Set(middleware.CtxUserRole, u.Role)
		c.Set(middleware.CtxUserTier, u.Tier)
		c.Next()
	}
}
