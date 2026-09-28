package handler

import (
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
)

func taskStartupHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.JSON(http.StatusOK, svc.StartupStatus())
	}
}

func requireTasksReady(c *gin.Context, svc *service.Container) bool {
	if svc.StartupStatus().State != "ready" {
		c.Header("Retry-After", "3")
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "startup_not_ready", "error": "服务尚未完成初始化，请查看任务中心启动进度"})
		return false
	}
	return true
}
