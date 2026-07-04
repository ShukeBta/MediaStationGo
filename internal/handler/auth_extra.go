// Package handler — auth surface beyond /login + /register:
//
//	POST /auth/refresh   — issue a fresh JWT for the current user
//	POST /auth/logout    — best-effort no-op (kept for parity)
//	PATCH /auth/profile  — alias for /me
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// refreshHandler returns a fresh token signed for the current user.
// Because we don't track refresh tokens server-side, the caller's
// existing access token is sufficient — it must already pass the
// AuthRequired middleware.
func refreshHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		u, err := svc.Repo.User.FindByID(c.Request.Context(), toString(uid))
		if err != nil || u == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			return
		}
		token, err := svc.Auth.IssueToken(u)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": token, "user": u})
	}
}

// logoutHandler is a deliberate no-op (we use stateless JWT). It exists
// so the Vue frontend's logout button gets a 200 instead of 404.
func logoutHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		clearAccessTokenCookie(c)
		c.Status(http.StatusNoContent)
	}
}
