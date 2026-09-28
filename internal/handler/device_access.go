package handler

import (
	"net/http"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func enforceTokenDevice(c *gin.Context, svc *service.Container, emby bool) bool {
	if svc.Device == nil {
		return true
	}
	id := c.GetString(middleware.CtxDeviceID)
	if id == "" && !emby && !c.GetBool(middleware.CtxLegacyDeviceToken) {
		return true
	}
	kicked, err := svc.Device.TokenDeviceKicked(c.Request.Context(), c.GetString(middleware.CtxUserID),
		id, c.GetString(middleware.CtxDeviceName), c.GetString(middleware.CtxDeviceClient))
	if err != nil {
		c.Header("Retry-After", "1")
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "device authorization temporarily unavailable"})
		return false
	}
	if kicked {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "device signed out; log in again"})
		return false
	}
	return true
}
