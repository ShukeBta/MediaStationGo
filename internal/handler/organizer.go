// Package handler — media file organizer endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func organizeMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		dst, err := svc.Organizer.OrganizeMedia(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"path": dst})
	}
}

func organizeLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		res, err := svc.Organizer.OrganizeLibrary(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	}
}
