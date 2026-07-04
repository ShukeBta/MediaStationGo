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
				c.Next()
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
		c.Next()
	}
}

func activeEmbyUserRequired(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		userID, _ := uid.(string)
		u, err := svc.Repo.User.FindByID(c.Request.Context(), userID)
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"Code": 40101, "Message": "User not found"})
			return
		}
		if err != nil {
			if service.IsTransientDatabaseLock(err) {
				c.Next()
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
		c.Next()
	}
}
