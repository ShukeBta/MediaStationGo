// Package handler — multi-persona play profile CRUD endpoints.
//
// Every user sees / mutates only their own profiles. Admin user
// management belongs in a separate admin surface, not this switcher.
package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type verifyPlayProfilePINReq struct {
	PIN string `json:"pin"`
}

type deletePlayProfileReq struct {
	PIN      string `json:"pin"`
	Password string `json:"password"`
}

// listPlayProfilesHandler returns only the caller's own profiles.
func listPlayProfilesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		rows, err := svc.PlayProfiles.ListByUser(c.Request.Context(), toString(uid))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rows)
	}
}

func createPlayProfileHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in service.PlayProfileInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		in.UserID = toString(uid)
		row, err := svc.PlayProfiles.Create(c.Request.Context(), in)
		if errors.Is(err, service.ErrPlayProfileLimit) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "每个用户最多只能创建 3 个观影 Profile"})
			return
		}
		if errors.Is(err, service.ErrPlayProfileValidation) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, row)
	}
}

func updatePlayProfileHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in service.PlayProfileInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		row, err := svc.PlayProfiles.UpdateForUser(c.Request.Context(), c.Param("id"), toString(uid), in)
		if errors.Is(err, service.ErrPlayProfileNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}
		if errors.Is(err, service.ErrPlayProfileForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "profile forbidden"})
			return
		}
		if errors.Is(err, service.ErrPlayProfileValidation) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, row)
	}
}

func deletePlayProfileHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		userID := toString(uid)
		var req deletePlayProfileReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		profile, err := svc.Repo.PlayProfile.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if profile == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}
		if profile.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "profile forbidden"})
			return
		}
		verified := false
		if profile.RequirePIN && req.PIN != "" {
			if _, err := svc.PlayProfiles.VerifyPIN(c.Request.Context(), profile.ID, userID, req.PIN); err == nil {
				verified = true
			}
		}
		if !verified && req.Password != "" {
			if err := svc.Auth.VerifyPassword(c.Request.Context(), userID, req.Password); err == nil {
				verified = true
			}
		}
		if !verified {
			if profile.RequirePIN {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "删除此 Profile 需要输入 PIN 或当前账号密码"})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "删除此 Profile 需要输入当前账号密码"})
			}
			return
		}

		if err := svc.PlayProfiles.DeleteForUser(c.Request.Context(), profile.ID, userID); errors.Is(err, service.ErrPlayProfileNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		} else if errors.Is(err, service.ErrPlayProfileForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "profile forbidden"})
			return
		} else if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func verifyPlayProfilePINHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req verifyPlayProfilePINReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		profile, err := svc.PlayProfiles.VerifyPIN(c.Request.Context(), c.Param("id"), toString(uid), req.PIN)
		if errors.Is(err, service.ErrPlayProfileNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
			return
		}
		if errors.Is(err, service.ErrPlayProfileForbidden) {
			c.JSON(http.StatusForbidden, gin.H{"error": "profile forbidden"})
			return
		}
		if errors.Is(err, service.ErrPlayProfilePINInvalid) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "PIN 错误"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		expiresAt := time.Now().Add(12 * time.Hour)
		token := signPlayProfilePINToken(svc, toString(uid), profile.ID, expiresAt)
		c.JSON(http.StatusOK, gin.H{
			"profile":    profile,
			"token":      token,
			"expires_at": expiresAt.Format(time.RFC3339),
		})
	}
}
