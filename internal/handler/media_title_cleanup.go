package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type mediaTitleCleanupPreviewRequest struct {
	GroupLimit int `json:"group_limit"`
}

func previewMediaTitleCleanupHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req mediaTitleCleanupPreviewRequest
		if c.Request.ContentLength > 0 {
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
		}
		job, err := svc.Media.StartTitleCleanupJob(svc.Context(), c.Param("id"), req.GroupLimit)
		if err != nil {
			writeMediaTitleCleanupError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, job)
	}
}

func getMediaTitleCleanupJobHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		job, err := svc.Media.GetTitleCleanupJob(c.Param("id"), c.Param("job_id"))
		if err != nil {
			writeMediaTitleCleanupError(c, err)
			return
		}
		c.JSON(http.StatusOK, job)
	}
}

func applyMediaTitleCleanupHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req service.MediaTitleCleanupApplyRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := svc.Media.ApplyTitleCleanup(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			writeMediaTitleCleanupError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func writeMediaTitleCleanupError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "媒体库不存在"})
	case errors.Is(err, service.ErrMediaTitleCleanupJobNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrAITitleCleanupUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrMediaTitleCleanupLibraryMode), errors.Is(err, service.ErrMediaTitleCleanupNoCandidates):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
}
