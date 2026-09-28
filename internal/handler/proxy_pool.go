package handler

import (
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

func listProxyPoolHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.ProxyPool.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func replaceProxyPoolHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			Items []service.ProxyPoolInput `json:"items"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		items, err := svc.ProxyPool.Replace(c.Request.Context(), request.Items)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func getProxyPoolConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		config, err := svc.ProxyPool.GetConfig(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "proxy pool configuration unavailable"})
			return
		}
		c.JSON(http.StatusOK, config)
	}
}

func updateProxyPoolConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var patch service.ProxyPoolConfigPatch
		if err := c.ShouldBindJSON(&patch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		config, err := svc.ProxyPool.UpdateConfig(c.Request.Context(), patch)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, config)
	}
}

func checkProxyPoolHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		result, err := svc.ProxyPool.Check(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "proxy pool check failed"})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func cleanupProxyPoolHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var request struct {
			Token string `json:"token"`
		}
		if err := c.ShouldBindJSON(&request); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := svc.ProxyPool.Cleanup(c.Request.Context(), request.Token)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "proxy pool cleanup failed; run the check again"})
			return
		}
		c.JSON(http.StatusOK, result)
	}
}
