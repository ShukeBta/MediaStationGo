// Package handler — download client (qBittorrent / Aria2 / Transmission)
// configuration endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func listDownloadClientsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := svc.DownloadClients.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if rows == nil {
			rows = []model.DownloadClient{}
		}
		c.JSON(http.StatusOK, rows)
	}
}

func createDownloadClientHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in service.DownloadClientInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		row, err := svc.DownloadClients.Create(c.Request.Context(), in)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// 让真正发起下载的 DownloadService 立刻读到新的 qb 配置，
		// 避免保存后还要重启进程才能生效。
		_ = svc.Downloads.ReloadConfig(c.Request.Context())
		c.JSON(http.StatusCreated, row)
	}
}

func updateDownloadClientHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in service.DownloadClientInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		row, err := svc.DownloadClients.Update(c.Request.Context(), c.Param("id"), in)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_ = svc.Downloads.ReloadConfig(c.Request.Context())
		c.JSON(http.StatusOK, row)
	}
}

func deleteDownloadClientHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.DownloadClients.Delete(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		_ = svc.Downloads.ReloadConfig(c.Request.Context())
		c.Status(http.StatusNoContent)
	}
}

func testDownloadClientHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.DownloadClients.Test(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func aria2StatsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := c.Query("client_id")
		if clientID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "client_id required"})
			return
		}
		out, err := svc.DownloadClients.Aria2GlobalStats(c.Request.Context(), clientID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, out)
	}
}
