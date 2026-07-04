package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func recognitionWordsService(svc *service.Container) *service.RecognitionWordsService {
	if svc == nil {
		return nil
	}
	if svc.RecognitionWords != nil {
		return svc.RecognitionWords
	}
	return service.NewRecognitionWordsService(svc.Log, svc.Repo)
}

func getRecognitionWordsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rw := recognitionWordsService(svc)
		if rw == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "recognition words service unavailable"})
			return
		}
		c.JSON(http.StatusOK, rw.Config(c.Request.Context()))
	}
}

func saveRecognitionWordsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rw := recognitionWordsService(svc)
		if rw == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "recognition words service unavailable"})
			return
		}
		var req service.RecognitionWordsConfig
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := rw.SaveConfig(c.Request.Context(), req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rw.Config(c.Request.Context()))
	}
}

func syncRecognitionWordsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rw := recognitionWordsService(svc)
		if rw == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "recognition words service unavailable"})
			return
		}
		cfg, err := rw.SyncShared(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, cfg)
	}
}

func testRecognitionWordsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rw := recognitionWordsService(svc)
		if rw == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "recognition words service unavailable"})
			return
		}
		var req struct {
			Input string `json:"input"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rw.Test(c.Request.Context(), req.Input))
	}
}
