// Package handler — scheduled jobs admin page.
package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func schedulerStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"jobs": svc.Scheduler.Status()})
	}
}

func schedulerRunHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		if !triggerSchedulerJob(c, svc, name) {
			return
		}
		c.JSON(http.StatusAccepted, gin.H{"ok": true, "message": "任务已在后台触发"})
	}
}

func triggerSchedulerJob(c *gin.Context, svc *service.Container, name string) bool {
	if !requireTasksReady(c, svc) {
		return false
	}
	if svc.Scheduler == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "任务调度器不可用"})
		return false
	}
	if err := svc.Scheduler.RunNowAsync(c.Request.Context(), name); err != nil {
		switch {
		case errors.Is(err, service.ErrSchedulerJobAlreadyRunning), errors.Is(err, service.ErrLocalScanAlreadyRunning):
			c.JSON(http.StatusConflict, gin.H{"error": "任务正在运行，请稍后到实时任务查看进度"})
		case errors.Is(err, service.ErrSchedulerJobNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "任务不存在"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return false
	}
	return true
}
