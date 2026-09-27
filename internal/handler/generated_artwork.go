package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"gorm.io/gorm"
)

type generateMediaArtworkAtRequest struct {
	TimestampSeconds float64 `json:"timestamp_seconds" binding:"required"`
}

func generatedArtworkStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.GeneratedArtwork == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "generated artwork service unavailable"})
			return
		}
		status, err := svc.GeneratedArtwork.Status(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, status)
	}
}

func runGeneratedArtworkHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.GeneratedArtwork == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "generated artwork service unavailable"})
			return
		}
		queued, err := svc.GeneratedArtwork.QueueMissingForLibrary(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"queued": queued})
	}
}

func cancelGeneratedArtworkHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.GeneratedArtwork == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "generated artwork service unavailable"})
			return
		}
		canceled, err := svc.GeneratedArtwork.CancelLibrary(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"canceled": canceled})
	}
}

func generateMediaArtworkAtHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.GeneratedArtwork == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "generated artwork service unavailable"})
			return
		}
		var req generateMediaArtworkAtRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		media, err := svc.GeneratedArtwork.GenerateAtForMedia(c.Request.Context(), c.Param("id"), req.TimestampSeconds)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, media)
	}
}
