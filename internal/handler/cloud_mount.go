package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

// cloudMountHandler creates or reuses a cloud:// media library for a cloud
// directory, then queues a recursive import scan. The scan runs outside the
// request so large 115/OpenList folders do not make the UI report a timeout.
func cloudMountHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		typ := c.Param("type")
		if !service.IsAdminCloudConfigurable(typ) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported cloud provider"})
			return
		}
		var in struct {
			Dir       string `json:"dir"`
			DirPath   string `json:"dir_path"`
			Name      string `json:"name"`
			MediaType string `json:"media_type"`
		}
		_ = c.ShouldBindJSON(&in)
		if !cloud.IsCloudType(typ) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported cloud provider"})
			return
		}
		if _, err := svc.StorageCfg.CloudProvider(c.Request.Context(), typ); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		path := service.BuildCloudLibraryPath(typ, in.Dir, in.DirPath)
		name := strings.TrimSpace(in.Name)
		if name == "" {
			name = cloudMountLibraryName(typ, strings.TrimSpace(in.Dir), strings.TrimSpace(in.DirPath))
		}
		mediaType := strings.TrimSpace(in.MediaType)
		if mediaType == "" || strings.EqualFold(mediaType, "auto") {
			displayDir := strings.TrimSpace(in.DirPath)
			if displayDir == "" {
				displayDir = strings.TrimSpace(in.Dir)
			}
			mediaType = service.InferCloudMountMediaType(displayDir, name)
		}
		libs, err := svc.Repo.Library.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		var lib *model.Library
		alreadyMounted := false
		if conflict := service.FindCloudMountConflict(libs, typ, in.Dir, in.DirPath); conflict != nil {
			lib = &conflict.Library
			alreadyMounted = conflict.Exact
			if conflict.Nested {
				c.JSON(http.StatusOK, gin.H{
					"library":          lib,
					"skipped":          true,
					"reason":           "cloud mount overlaps an existing mounted parent/child directory",
					"conflict_library": conflict.Library,
				})
				return
			}
		}
		if lib == nil {
			lib = &model.Library{Name: name, Path: path, Type: mediaType, Enabled: true}
			if err := svc.Repo.Library.Create(c.Request.Context(), lib); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		} else if alreadyMounted {
			updates := map[string]any{}
			if path != "" && path != lib.Path {
				updates["path"] = path
				lib.Path = path
			}
			if mediaType != "" && mediaType != lib.Type {
				updates["type"] = mediaType
				lib.Type = mediaType
			}
			currentDisplayName, _ := service.CloudLibraryDisplayName(*lib)
			if name != "" && name != lib.Name && (currentDisplayName == "" || currentDisplayName != name || strings.Contains(lib.Name, " · ")) {
				updates["name"] = name
				lib.Name = name
			}
			if len(updates) > 0 {
				if err := svc.Repo.DB.WithContext(c.Request.Context()).Model(&model.Library{}).Where("id = ?", lib.ID).Updates(updates).Error; err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
			}
		}
		if svc.Scan != nil {
			libID := lib.ID
			if svc.WSHub != nil {
				svc.WSHub.Publish("scan", gin.H{
					"library_id":       libID,
					"cloud":            true,
					"queued":           true,
					"stage":            "queued",
					"message":          "云盘扫描已加入后台队列，会递归扫描并自动加入媒体库",
					"estimate_message": "小目录通常几十秒；几万文件的大目录可能需要数分钟到数小时，取决于网盘接口速度",
				})
			}
			_, _, _ = svc.Scan.StartCloudLibraryScan(libID, false)
		}
		c.JSON(http.StatusAccepted, gin.H{
			"library":          lib,
			"already_mounted":  alreadyMounted,
			"scan_queued":      svc.Scan != nil,
			"message":          "挂载后会后台递归扫描，发现的媒体会自动加入当前媒体库",
			"estimate_message": "小目录通常几十秒；几万文件的大目录可能需要数分钟到数小时，取决于网盘接口速度",
		})
	}
}

func cloudMountLibraryName(typ, dir, displayDir string) string {
	base := service.CloudMountProviderLabel(typ)
	displayDir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(displayDir, "\\", "/")), "/")
	if displayDir != "" {
		parts := strings.Split(displayDir, "/")
		for i := len(parts) - 1; i >= 0; i-- {
			if part := strings.TrimSpace(parts[i]); part != "" {
				return part
			}
		}
	}
	if dir == "" || dir == "0" {
		return base
	}
	dir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(dir, "\\", "/")), "/")
	if dir == "" {
		return base
	}
	parts := strings.Split(dir, "/")
	for i := len(parts) - 1; i >= 0; i-- {
		if part := strings.TrimSpace(parts[i]); part != "" {
			return part
		}
	}
	return base
}
