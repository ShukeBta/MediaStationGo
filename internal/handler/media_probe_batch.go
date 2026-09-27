package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func probeMissingMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Stream == nil || svc.FFprobe == nil || svc.Tasks == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "media probe task service unavailable"})
			return
		}
		maxConcurrent := svc.FFprobe.MaxConcurrent()
		task, started := svc.Tasks.StartUnique(service.TaskKindProbe, "探测未完成媒体轨", service.TaskUpdate{
			Stage:   "queued",
			Message: fmt.Sprintf("媒体轨探测任务已排队，并发数 %d", maxConcurrent),
		})
		if !started {
			c.JSON(http.StatusAccepted, gin.H{"queued": false, "already_running": true})
			return
		}

		go func() {
			result, err := svc.Stream.ProbeMissingMedia(context.Background(), svc.FFprobe, maxConcurrent, func(progress service.MediaProbeBatchResult) {
				task.Update(service.TaskUpdate{
					Stage:   "probing",
					Message: fmt.Sprintf("正在探测媒体轨：%d/%d", progress.Probed+progress.Failed, progress.Total),
					Metrics: mediaProbeTaskMetrics(progress),
					Details: progress.Errors,
				})
			})
			message := fmt.Sprintf("媒体轨探测完成：成功 %d，失败 %d", result.Probed, result.Failed)
			finishHTTPTask(task, err, "completed", message, mediaProbeTaskMetrics(result), result.Errors)
		}()

		c.JSON(http.StatusAccepted, gin.H{
			"queued":         true,
			"task_id":        task.ID(),
			"max_concurrent": maxConcurrent,
		})
	}
}

func mediaProbeTaskMetrics(result service.MediaProbeBatchResult) map[string]int64 {
	return map[string]int64{
		"queued": int64(result.Total),
		"probed": int64(result.Probed),
		"errors": int64(result.Failed),
	}
}
