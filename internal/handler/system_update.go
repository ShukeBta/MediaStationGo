package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func systemUpdateStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.SystemUpdate == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "system update service unavailable"})
			return
		}
		c.JSON(http.StatusOK, svc.SystemUpdate.Status(c.Request.Context()))
	}
}

func systemUpdateCheckHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.SystemUpdate == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "system update service unavailable"})
			return
		}
		c.JSON(http.StatusOK, svc.SystemUpdate.Check(c.Request.Context()))
	}
}

func systemUpdateApplyHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.SystemUpdate == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "system update service unavailable"})
			return
		}
		status, err := svc.SystemUpdate.Apply(c.Request.Context())
		if err != nil {
			if errors.Is(err, service.ErrSystemUpdateRunning) {
				c.JSON(http.StatusConflict, gin.H{"error": err.Error(), "status": status})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "status": status})
			return
		}
		c.JSON(http.StatusAccepted, status)
	}
}
