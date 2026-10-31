package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func registerAdminPluginRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/plugins", listPluginsHandler(svc))
	admin.PATCH("/plugins/:id", updatePluginHandler(svc))
	admin.POST("/plugins/:id/run", runPluginHandler(svc))
}

func pluginServiceAvailable(c *gin.Context, svc *service.Container) bool {
	if svc == nil || svc.Plugins == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "插件服务暂不可用"})
		return false
	}
	return true
}

func listPluginsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !pluginServiceAvailable(c, svc) {
			return
		}
		items, err := svc.Plugins.List(c.Request.Context())
		if err != nil {
			writePluginError(c, svc, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func updatePluginHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !pluginServiceAvailable(c, svc) {
			return
		}
		var update service.PluginUpdate
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&update); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "插件配置格式不正确"})
			return
		}
		if decoder.Decode(new(any)) != io.EOF || (update.Enabled == nil && update.Config == nil) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "请提供启用状态或配置对象"})
			return
		}
		item, err := svc.Plugins.Update(c.Request.Context(), c.Param("id"), update)
		if err != nil {
			writePluginError(c, svc, err)
			return
		}
		c.JSON(http.StatusOK, item)
	}
}

func runPluginHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !pluginServiceAvailable(c, svc) {
			return
		}
		ctx, cancel := context.WithCancel(c.Request.Context())
		stop := context.AfterFunc(svc.Context(), cancel)
		defer func() { stop(); cancel() }()
		if svc.Context().Err() != nil {
			cancel()
		}
		result, err := svc.Plugins.Run(ctx, c.Param("id"))
		if err != nil {
			writePluginError(c, svc, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func writePluginError(c *gin.Context, svc *service.Container, err error) {
	status, message := http.StatusInternalServerError, "插件操作失败，请稍后重试"
	switch {
	case errors.Is(err, service.ErrPluginNotFound):
		status, message = http.StatusNotFound, "插件不存在"
	case errors.Is(err, service.ErrPluginDisabled):
		status, message = http.StatusConflict, "请先启用插件"
	case errors.Is(err, service.ErrPluginBusy):
		status, message = http.StatusConflict, "插件正在运行，请稍后重试"
	case errors.Is(err, service.ErrPluginInvalidConfig):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, context.DeadlineExceeded):
		status, message = http.StatusGatewayTimeout, "插件运行超时"
	case errors.Is(err, context.Canceled):
		status, message = http.StatusServiceUnavailable, "插件运行已取消"
	}
	if status >= 500 && svc.Log != nil {
		svc.Log.Warn("plugin operation failed", zap.Error(err))
	}
	c.JSON(status, gin.H{"error": message})
}
