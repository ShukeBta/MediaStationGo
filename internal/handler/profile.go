// Package handler — user profile endpoints.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func updateProfileHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var patch service.ProfileUpdate
		if err := c.ShouldBindJSON(&patch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		userID := uid.(string)
		hideAdultChanged, err := profileHideAdultChanged(c.Request.Context(), svc, userID, patch)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		usernameChanged, err := profileUsernameChanged(c.Request.Context(), svc, userID, patch)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if hideAdultChanged || usernameChanged {
			if err := svc.Auth.VerifyPassword(c.Request.Context(), userID, patch.Password); err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "需要输入当前账号密码确认"})
				return
			}
		}
		u, err := svc.Profile.UpdateProfile(c.Request.Context(), userID, patch)
		if err != nil {
			if errors.Is(err, service.ErrAdultContentBlockedByAdmin) {
				c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
				return
			}
			if errors.Is(err, service.ErrUsernameTaken) {
				c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, u)
	}
}

func profileHideAdultChanged(ctx context.Context, svc *service.Container, userID string, patch service.ProfileUpdate) (bool, error) {
	if patch.HideAdult == nil {
		return false, nil
	}
	user, err := svc.Repo.User.FindByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, errors.New("user not found")
	}
	return user.HideAdult != *patch.HideAdult, nil
}

func profileUsernameChanged(ctx context.Context, svc *service.Container, userID string, patch service.ProfileUpdate) (bool, error) {
	if patch.Username == nil {
		return false, nil
	}
	user, err := svc.Repo.User.FindByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if user == nil {
		return false, errors.New("user not found")
	}
	return user.Username != strings.TrimSpace(*patch.Username), nil
}

type adminUpdateRoleReq struct {
	Role string `json:"role" binding:"required"`
}

func adminUpdateRoleHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminUpdateRoleReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		u, err := svc.Profile.AdminUpdateRole(c.Request.Context(), c.Param("id"), req.Role)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, u)
	}
}
