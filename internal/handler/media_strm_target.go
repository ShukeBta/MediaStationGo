package handler

import (
	"errors"
	"net/http"
	"os"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

// Storage paths and potentially signed source URLs are admin-only, as in
// mediaForResponse. Playback continues to use media IDs for ordinary users.
func getMediaSTRMTargetHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		target, err := svc.Media.GetSTRMTarget(c.Request.Context(), c.Param("id"))
		if err != nil || target == "" {
			c.JSON(http.StatusNotFound, gin.H{"error": "STRM target not found"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"target": target})
	}
}

func previewSTRMDeleteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := svc.FileManager.ResolveSTRMDeleteTarget(c.Request.Context(), c.Param("id"))
		writeSTRMDeleteResponse(c, result, err)
	}
}

func deleteSTRMTargetHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			DeleteParent bool   `json:"delete_parent"`
			Confirmation string `json:"confirmation" binding:"required"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Preview the target before deleting it"})
			return
		}
		path, err := svc.FileManager.DeleteSTRMTarget(c.Request.Context(), c.Param("id"), req.DeleteParent, req.Confirmation)
		writeSTRMDeleteResponse(c, gin.H{"removed": err == nil, "path": path}, err)
	}
}

func writeSTRMDeleteResponse(c *gin.Context, result any, err error) {
	if errors.Is(err, service.ErrSTRMTargetChanged) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, service.ErrMediaNotFound) || errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusNotFound, gin.H{"error": "STRM target not found"})
		return
	}
	writeFileManagerResponse(c, result, err)
}
