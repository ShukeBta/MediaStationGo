// Package handler — library / media HTTP endpoints.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type createLibraryReq struct {
	Name string `json:"name" binding:"required"`
	Path string `json:"path" binding:"required"`
	Type string `json:"type"`
}

func listLibrariesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libs, err := svc.Media.ListLibraries(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		role, _ := c.Get(middleware.CtxUserRole)
		includeHidden := role == "admin" && (c.Query("include_hidden") == "1" || c.Query("all") == "1")
		if !includeHidden {
			libs = service.FilterDisplayCloudLibraries(c.Request.Context(), svc.Repo, libs)
			visibility := mediaVisibilityForRequest(c, svc)
			filtered := libs[:0]
			for _, lib := range libs {
				if service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, lib, visibility) {
					filtered = append(filtered, lib)
				}
			}
			libs = filtered
		} else {
			libs = service.NormalizeCloudLibraryDisplayNames(libs)
		}
		c.JSON(http.StatusOK, libs)
	}
}

func createLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createLibraryReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		l, err := svc.Media.CreateLibrary(c.Request.Context(), req.Name, req.Path, req.Type)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get("ctx_user_id")
		svc.Audit.Record(c.Request.Context(), toString(uid), "library.create", l.ID, c.ClientIP(), l.Path)
		// Refresh fsnotify watcher to pick up the new library root.
		go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		c.JSON(http.StatusCreated, l)
	}
}

func deleteLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if lib, err := svc.Repo.Library.FindByID(c.Request.Context(), id); err == nil && lib != nil {
			if _, ok := service.ParseCloudLibraryMount(lib.Path); ok && svc.Scan != nil {
				_ = svc.Scan.CancelCloudScan(id)
			}
		}
		if err := svc.Media.DeleteLibrary(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get("ctx_user_id")
		svc.Audit.Record(c.Request.Context(), toString(uid), "library.delete", id, c.ClientIP(), "")
		go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		c.Status(http.StatusNoContent)
	}
}

func scanLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		lib, err := svc.Repo.Library.FindByID(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if lib == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "library not found"})
			return
		}
		if _, ok := service.ParseCloudLibraryMount(lib.Path); ok {
			task := startScanHTTPTask(svc, "云盘扫描队列", lib.Name, lib.Path)
			if svc.WSHub != nil {
				svc.WSHub.Publish("scan", gin.H{
					"library_id":       id,
					"cloud":            true,
					"queued":           true,
					"stage":            "queued",
					"message":          "云盘扫描已加入后台队列，会递归扫描并自动加入媒体库",
					"estimate_message": "小目录通常几十秒；几万文件的大目录可能需要数分钟到数小时，取决于网盘接口速度",
				})
			}
			_, _, _ = svc.Scan.StartCloudLibraryScan(id, false)
			finishHTTPTask(task, nil, "queued", "云盘扫描已加入后台队列", map[string]int64{"queued": 1}, nil)
			c.JSON(http.StatusAccepted, gin.H{
				"library_id":       id,
				"visited":          0,
				"added":            0,
				"updated":          0,
				"probed":           0,
				"queued":           true,
				"cloud":            true,
				"message":          "云盘扫描已在后台运行，发现的媒体会自动加入当前媒体库",
				"estimate_message": "小目录通常几十秒；几万文件的大目录可能需要数分钟到数小时，取决于网盘接口速度",
			})
			return
		}
		finishScan, ok := svc.Scan.TryBeginLocalScan(id)
		if !ok {
			c.JSON(http.StatusAccepted, gin.H{
				"library_id":       id,
				"queued":           true,
				"already_running":  true,
				"message":          "该媒体库正在后台扫描，请在任务面板查看进度",
				"estimate_message": "页面关闭不会中断扫描",
			})
			return
		}
		task := startScanHTTPTask(svc, "手动扫描入库", lib.Name, lib.Path)
		go func(libraryID string, task *service.TaskHandle, finish func()) {
			defer finish()
			res, err := svc.Scan.ScanLibrary(context.Background(), libraryID)
			if err != nil {
				finishHTTPTask(task, err, "scan", "手动扫描入库失败", scanTaskMetrics(res), scanTaskDetails(res, 20))
				return
			}
			finishHTTPTask(task, nil, "completed", "手动扫描入库结束", scanTaskMetrics(res), scanTaskDetails(res, 20))
		}(id, task, finishScan)
		c.JSON(http.StatusAccepted, gin.H{
			"library_id":       id,
			"queued":           true,
			"message":          "本地媒体库扫描已在后台运行，页面关闭不会中断",
			"estimate_message": "可在右上角任务面板查看扫描进度",
		})
	}
}

func startScanHTTPTask(svc *service.Container, name, libraryName, path string) *service.TaskHandle {
	if svc == nil || svc.Tasks == nil {
		return nil
	}
	if libraryName != "" {
		name += "：" + libraryName
	}
	return svc.Tasks.Start(service.TaskKindScan, name, service.TaskUpdate{
		Stage:      "scan",
		SourcePath: path,
		Message:    "正在扫描并入库",
	})
}

func scanTaskMetrics(res *service.ScanResult) map[string]int64 {
	if res == nil {
		return nil
	}
	return map[string]int64{
		"visited":        int64(res.Visited),
		"added":          int64(res.Added),
		"updated":        int64(res.Updated),
		"skipped":        int64(res.Skipped),
		"probed":         int64(res.Probed),
		"local_metadata": int64(res.LocalMetadata),
		"removed":        res.Removed,
		"errors":         int64(res.ErrorCount),
	}
}

func scanTaskDetails(res *service.ScanResult, limit int) []string {
	if res == nil || limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	for _, line := range res.Errors {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, "错误: "+line)
		if len(out) >= limit {
			return out
		}
	}
	return out
}

func listMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
		groupVersions := c.DefaultQuery("group_versions", "0") == "1"
		if !groupVersions {
			items, total, err := svc.Media.ListMediaVisible(c.Request.Context(), id, page, size, mediaVisibilityForRequest(c, svc))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"items":     items,
				"total":     total,
				"page":      page,
				"page_size": size,
			})
			return
		}
		items, total, err := svc.Media.ListMediaVisibleGrouped(c.Request.Context(), id, page, size, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": size,
		})
	}
}

func getMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		m, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if m == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if !mediaVisibleForRequest(c, svc, m) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, m)
	}
}

func updateMediaMetadataHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req service.MediaMetadataUpdate
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		m, err := svc.Media.UpdateMetadata(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				status = http.StatusNotFound
			} else if strings.Contains(strings.ToLower(err.Error()), "required") {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, m)
	}
}

func searchMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Query("q")
		groupVersions := c.DefaultQuery("group_versions", "0") == "1"
		if c.Query("page") != "" || c.Query("page_size") != "" {
			page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
			size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
			if !groupVersions {
				items, total, err := svc.Media.SearchMediaVisiblePage(c.Request.Context(), q, page, size, mediaVisibilityForRequest(c, svc))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"items":     items,
					"total":     total,
					"page":      page,
					"page_size": size,
				})
				return
			}
			items, total, err := svc.Media.SearchMediaVisiblePageGrouped(c.Request.Context(), q, page, size, mediaVisibilityForRequest(c, svc))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{
				"items":     items,
				"total":     total,
				"page":      page,
				"page_size": size,
			})
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if !groupVersions {
			items, err := svc.Media.SearchMediaVisible(c.Request.Context(), q, limit, mediaVisibilityForRequest(c, svc))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"items": items})
			return
		}
		items, err := svc.Media.SearchMediaVisibleGrouped(c.Request.Context(), q, limit, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func streamHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		m, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || m == nil || !mediaVisibleForRequest(c, svc, m) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if !enforceScopedPlaybackToken(c, m.ID) {
			return
		}
		err = svc.Stream.ServeFileWithCloudMode(c.Writer, c.Request, c.Param("id"), service.CloudPlaybackModeSTRM)
		if errors.Is(err, service.ErrMediaNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if errors.Is(err, service.ErrCloudPlaybackDisabled) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
}
