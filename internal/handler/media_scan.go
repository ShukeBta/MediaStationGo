package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

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
			_, _, _ = svc.Scan.StartCloudLibraryScan(id, true)
			finishHTTPTask(task, nil, "queued", "云盘扫描已加入后台队列", map[string]int64{"queued": 1}, nil)
			c.JSON(http.StatusAccepted, gin.H{
				"library_id":       id,
				"visited":          0,
				"added":            0,
				"updated":          0,
				"probed":           0,
				"queued":           true,
				"cloud":            true,
				"message":          "云盘扫描已在后台运行，发现的媒体会自动加入当前媒体库；若已开启自动刮削，会在扫描后补齐元数据",
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

func scanLibraryRootHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		rootID := c.Param("root_id")
		finishScan, ok := svc.Scan.TryBeginLocalScan(id + ":" + rootID)
		if !ok {
			c.JSON(http.StatusAccepted, gin.H{
				"library_id":       id,
				"queued":           true,
				"already_running":  true,
				"message":          "该路径正在后台扫描，请在任务面板查看进度",
				"estimate_message": "页面关闭不会中断扫描",
			})
			return
		}
		task := startScanHTTPTask(svc, "手动扫描媒体库路径", id, rootID)
		go func(libraryID, libraryRootID string, task *service.TaskHandle, finish func()) {
			defer finish()
			res, err := svc.Scan.ScanLibraryRoot(context.Background(), libraryID, libraryRootID)
			if err != nil {
				finishHTTPTask(task, err, "scan", "手动扫描路径失败", scanTaskMetrics(res), scanTaskDetails(res, 20))
				return
			}
			finishHTTPTask(task, nil, "completed", "手动扫描路径结束", scanTaskMetrics(res), scanTaskDetails(res, 20))
		}(id, rootID, task, finishScan)
		c.JSON(http.StatusAccepted, gin.H{
			"library_id":       id,
			"queued":           true,
			"message":          "媒体库路径扫描已在后台运行，页面关闭不会中断",
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
