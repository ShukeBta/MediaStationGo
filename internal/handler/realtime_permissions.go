package handler

import (
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

// Recheck permissions when delivering, since connections can outlive a role
// change. Unknown topics are private by default; task payloads contain paths.
func realtimeEventAllowed(c *gin.Context, svc *service.Container, topic string) bool {
	uid := c.GetString(middleware.CtxUserID)
	if uid == "" || svc.Repo == nil || svc.Repo.User == nil {
		return false
	}
	u, err := svc.Repo.User.FindByID(c.Request.Context(), uid)
	if err != nil || u == nil || !u.IsActive || (u.ExpiredAt != nil && !u.ExpiredAt.After(time.Now())) {
		return false
	}
	if u.Role == "admin" {
		return true
	}
	if topic != "download" || svc.Permissions == nil {
		return false
	}
	permissions, err := svc.Permissions.Effective(c.Request.Context(), uid)
	return err == nil && permissions != nil && permissions.CanManageDownloads
}
