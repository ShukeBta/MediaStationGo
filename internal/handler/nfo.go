// Package handler — NFO export endpoints (Kodi / Jellyfin compatibility).
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func exportNFOHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		path, err := svc.NFO.ExportOne(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"path": path})
	}
}

func nfoEditTargetHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		target, err := svc.Media.NFOEditTarget(c.Request.Context(), c.Param("id"), c.Query("scope"))
		writeFileManagerResponse(c, target, err)
	}
}

func exportLibraryNFOHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		written, err := svc.NFO.ExportLibrary(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"written": written})
	}
}
