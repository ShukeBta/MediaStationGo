// Package handler — admin endpoints (users / settings / logs).
package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func listUsersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		users, err := svc.Repo.User.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := annotateProtectedUsers(c.Request.Context(), svc, users); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if svc.Sessions != nil {
			svc.Sessions.ApplyToUsers(c.Request.Context(), users)
		}
		c.JSON(http.StatusOK, users)
	}
}

type adminCreateUserReq struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func createUserHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminCreateUserReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		refreshLicenseCapacityBestEffort(c.Request.Context(), svc)
		u, _, err := svc.Auth.Register(c.Request.Context(), req.Username, req.Password)
		if err != nil {
			writeUserMutationError(c, svc, err)
			return
		}
		// Admin-created users are intentionally normal viewers by default.
		// They can log in from Web/Emby-compatible clients and play media, but
		// cannot scrape, scan, download, delete, export NFO, or manage files.
		if u.Role != "user" {
			u, err = svc.Profile.AdminUpdateRole(c.Request.Context(), u.ID, "user")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		c.JSON(http.StatusCreated, u)
	}
}

type adminUpdateUserReq struct {
	Username string `json:"username" binding:"required"`
}

type adminResetPasswordReq struct {
	Password string `json:"password" binding:"required,min=6"`
}

type adminUpdateUserStatusReq struct {
	IsActive bool `json:"is_active"`
}

func updateUserHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminUpdateUserReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		nextUsername := strings.TrimSpace(req.Username)
		if nextUsername == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "username required"})
			return
		}
		userID := c.Param("id")
		user, err := svc.Repo.User.FindByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if user == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
			return
		}
		if existing, err := svc.Repo.User.FindByUsername(c.Request.Context(), nextUsername); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		} else if existing != nil && existing.ID != userID {
			writeUserMutationError(c, svc, service.ErrUsernameTaken)
			return
		}
		updates := map[string]any{"username": nextUsername}
		if firstAdmin, err := svc.Repo.User.FirstAdmin(c.Request.Context()); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		} else if firstAdmin != nil && firstAdmin.ID == userID {
			updates["role"] = "admin"
			updates["tier"] = "plus"
		}
		if err := svc.Repo.User.UpdateFields(c.Request.Context(), userID, updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		updated, err := svc.Repo.User.FindByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, updated)
	}
}

func deleteUserHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		firstAdmin, err := svc.Repo.User.FirstAdmin(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if firstAdmin != nil && firstAdmin.ID == c.Param("id") {
			c.JSON(http.StatusForbidden, gin.H{"error": "default admin cannot be deleted"})
			return
		}
		if svc.Sessions != nil && svc.Sessions.UserRecentlyActive(c.Request.Context(), c.Param("id"), service.RealtimeDeletionGuardWindow()) {
			c.JSON(http.StatusConflict, gin.H{"error": "user has a recent realtime session; confirm the user is offline before deletion"})
			return
		}
		if err := svc.Repo.User.Delete(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func resetUserPasswordHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminResetPasswordReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := svc.Auth.ResetPassword(c.Request.Context(), c.Param("id"), req.Password); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func updateUserStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req adminUpdateUserStatusReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		userID := c.Param("id")
		if !req.IsActive {
			if firstAdmin, err := svc.Repo.User.FirstAdmin(c.Request.Context()); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			} else if firstAdmin != nil && firstAdmin.ID == userID {
				c.JSON(http.StatusForbidden, gin.H{"error": "default admin cannot be disabled"})
				return
			}
		}
		updates := map[string]any{"is_active": req.IsActive}
		if req.IsActive {
			updates["share_warnings"] = 0
			updates["last_share_warn_at"] = nil
		}
		if err := svc.Repo.User.UpdateFields(c.Request.Context(), userID, updates); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if req.IsActive {
			_ = svc.Repo.UserDevice.SetKickedByUser(c.Request.Context(), userID, false)
		} else {
			_ = svc.Repo.UserDevice.SetKickedByUser(c.Request.Context(), userID, true)
		}
		updated, err := svc.Repo.User.FindByID(c.Request.Context(), userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, updated)
	}
}

func annotateProtectedUsers(ctx context.Context, svc *service.Container, users []model.User) error {
	firstAdmin, err := svc.Repo.User.FirstAdmin(ctx)
	if err != nil {
		return err
	}
	for i := range users {
		if service.UserIsProtectedAccount(ctx, svc.Repo, &users[i]) {
			users[i].IsProtected = true
		}
		if firstAdmin != nil && users[i].ID == firstAdmin.ID {
			users[i].IsDefaultAdmin = true
			users[i].IsProtected = true
			users[i].Role = "admin"
			users[i].Tier = "plus"
		}
	}
	return nil
}

func writeUserMutationError(c *gin.Context, svc *service.Container, err error) {
	switch {
	case errors.Is(err, service.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
	case errors.Is(err, service.ErrUserLimitReached):
		maxUsers := service.LicensedMaxUsers(c.Request.Context(), svc.Repo)
		c.JSON(http.StatusBadRequest, gin.H{"error": "user limit reached", "max_users": maxUsers})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
