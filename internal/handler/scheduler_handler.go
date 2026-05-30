// Package handler — 定时任务管理 HTTP 端点。
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// SchedulerHandler 处理定时任务的查询和管理操作。
type SchedulerHandler struct {
	svc *service.Container
	log *zap.Logger
}

// NewSchedulerHandler 创建定时任务处理器。
func NewSchedulerHandler(svc *service.Container, log *zap.Logger) *SchedulerHandler {
	return &SchedulerHandler{svc: svc, log: log}
}

// ListTasks 返回所有定时任务列表。
func (h *SchedulerHandler) ListTasks(c *gin.Context) {
	tasks := h.svc.Scheduler.Status()
	Success(c, tasks)
}

// RunTask 手动触发指定任务。
func (h *SchedulerHandler) RunTask(c *gin.Context) {
	name := c.Param("id")
	ctx := c.Request.Context()

	if err := h.svc.Scheduler.RunNow(ctx, name); err != nil {
		Error(c, http.StatusInternalServerError, ErrInternal, "任务执行失败: "+err.Error())
		return
	}

	SuccessWithMessage(c, "任务已触发执行", nil)
}

// GetStatus 返回调度器运行状态。
func (h *SchedulerHandler) GetStatus(c *gin.Context) {
	status := h.svc.Scheduler.Status()
	Success(c, status)
}
