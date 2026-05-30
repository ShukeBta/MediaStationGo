// Package handler — Alist / S3 / WebDAV storage config endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// listStorageConfigsHandler returns the status overview used by the
// admin storage panel: every persisted backend with secrets redacted.
func listStorageConfigsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := svc.StorageCfg.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": rows})
	}
}

// getStorageConfigHandler returns one config (with the decrypted body).
func getStorageConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		row, err := svc.StorageCfg.Get(c.Request.Context(), c.Param("type"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if row == nil {
			c.JSON(http.StatusOK, gin.H{"type": c.Param("type"), "config": gin.H{}})
			return
		}
		c.JSON(http.StatusOK, row)
	}
}

// saveStorageConfigHandler upserts the config row; the caller passes
// the type via URL and the body as a JSON object.
func saveStorageConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in service.StorageInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		in.Type = c.Param("type")
		row, err := svc.StorageCfg.Save(c.Request.Context(), in)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, row)
	}
}

// testStorageConfigHandler probes an unsaved config.
func testStorageConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var in service.StorageInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		in.Type = c.Param("type")
		if err := svc.StorageCfg.Test(c.Request.Context(), in); err != nil {
			c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
