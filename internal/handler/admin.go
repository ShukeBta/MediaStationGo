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
		u, _, err := svc.Auth.Register(c.Request.Context(), req.Username, req.Password)
		if err != nil {
			writeUserMutationError(c, err)
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
			writeUserMutationError(c, service.ErrUsernameTaken)
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

func annotateProtectedUsers(ctx context.Context, svc *service.Container, users []model.User) error {
	firstAdmin, err := svc.Repo.User.FirstAdmin(ctx)
	if err != nil || firstAdmin == nil {
		return err
	}
	for i := range users {
		if users[i].ID == firstAdmin.ID {
			users[i].IsDefaultAdmin = true
			users[i].IsProtected = true
			users[i].Role = "admin"
			users[i].Tier = "plus"
		}
	}
	return nil
}

func writeUserMutationError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUsernameTaken):
		c.JSON(http.StatusConflict, gin.H{"error": "username already taken"})
	case errors.Is(err, service.ErrUserLimitReached):
		c.JSON(http.StatusBadRequest, gin.H{"error": "user limit reached: maximum 20 users"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

type settingReq struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

func listSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := svc.Repo.Setting.All(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, settings)
	}
}

func updateSettingHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req settingReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		oldValue := ""
		if req.Key == service.AdultLibraryIDsSettingKey {
			oldValue, _ = svc.Repo.Setting.Get(c.Request.Context(), req.Key)
		}
		if err := svc.Repo.Setting.Set(c.Request.Context(), req.Key, req.Value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		oldAdultLibraryIDs := service.DecodeAllowedLibraryIDs(oldValue)
		newAdultLibraryIDs := service.DecodeAllowedLibraryIDs(req.Value)
		if req.Key == service.AdultLibraryIDsSettingKey && len(oldAdultLibraryIDs) == 0 && len(newAdultLibraryIDs) > 0 {
			_ = svc.Repo.DB.WithContext(c.Request.Context()).Model(&model.User{}).Where("hide_adult = ?", false).Update("hide_adult", true).Error
		}
		service.ApplyRuntimeSetting(svc.Cfg, req.Key, req.Value)
		if req.Key == "transcode.enabled" && !svc.Cfg.Transcoder.Enabled {
			svc.Transcoder.StopAll()
		}
		if req.Key == "transcode.hw_enabled" || req.Key == "transcode.hw_accel" || req.Key == "transcoder.hardware_accel" || req.Key == "transcoder.encoder" {
			svc.Transcoder.StopAll()
		}
		c.Status(http.StatusNoContent)
	}
}

func recentLogsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := svc.Repo.Log.Recent(c.Request.Context(), 200)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rows)
	}
}
