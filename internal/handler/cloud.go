// Package handler — cloud-disk (网盘) endpoints: directory browsing, QR-code
// login, media import and 302 playback redirects.
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// cloudListHandler browses a configured cloud disk directory.
func cloudListHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		if !service.IsAdminCloudConfigurable(typ) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported cloud provider", "items": []any{}})
			return
		}
		dir := c.Query("dir")
		entries, err := svc.StorageCfg.CloudList(c.Request.Context(), typ, dir)
		if err != nil {
			c.JSON(http.StatusOK, gin.H{"error": err.Error(), "items": []any{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": entries})
	}
}

func cloudMkdirHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		if !service.IsAdminCloudConfigurable(typ) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported cloud provider"})
			return
		}
		var in struct {
			Dir  string `json:"dir"`
			Name string `json:"name" binding:"required"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		entry, err := svc.StorageCfg.CloudMkdir(c.Request.Context(), typ, in.Dir, in.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"entry": entry})
	}
}

func cloudRenameHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		if !service.IsAdminCloudConfigurable(typ) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported cloud provider"})
			return
		}
		var in struct {
			Ref  string `json:"ref" binding:"required"`
			Name string `json:"name" binding:"required"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		entry, err := svc.StorageCfg.CloudRename(c.Request.Context(), typ, in.Ref, in.Name)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"entry": entry})
	}
}

// cloudImportHandler turns a cloud file into a playable 302-backed media item.
func cloudImportHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		if !service.IsAdminCloudConfigurable(typ) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported cloud provider"})
			return
		}
		var in struct {
			Ref  string `json:"ref" binding:"required"`
			Name string `json:"name"`
			Size int64  `json:"size"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		m, err := svc.StorageCfg.CloudImport(c.Request.Context(), typ, in.Ref, in.Name, in.Size)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, m)
	}
}

func cloudScanAllHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.Scan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scanner unavailable"})
			return
		}
		statuses, err := svc.Scan.StartAllCloudLibraryScans()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, gin.H{
			"items":            statuses,
			"scan_queued":      true,
			"message":          "已开始扫描所有启用的网盘媒体库",
			"resume_message":   "中断后再次点击扫描会重新遍历，但已入库媒体会去重更新，只补齐缺失项。",
			"estimate_message": "小目录通常几十秒；几万文件的大目录可能需要数分钟到数小时，取决于网盘接口速度",
		})
	}
}

func cloudScanCancelHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.Scan == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "scanner unavailable"})
			return
		}
		libraryID := strings.TrimSpace(c.Query("library_id"))
		provider := strings.TrimSpace(c.Query("provider"))
		cancelled := 0
		if libraryID != "" {
			if svc.Scan.CancelCloudScan(libraryID) {
				cancelled = 1
			}
		} else if provider != "" {
			cancelled = svc.Scan.CancelCloudScansForProvider(provider)
		} else {
			cancelled = svc.Scan.CancelAllCloudScans()
		}
		c.JSON(http.StatusOK, gin.H{
			"cancelled": cancelled,
			"message":   "已发送中断信号；正在等待当前网盘请求返回后停止",
		})
	}
}

func cloudScanStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.Scan == nil {
			c.JSON(http.StatusOK, gin.H{"items": []service.CloudScanStatus{}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": svc.Scan.CloudScanStatuses()})
	}
}
