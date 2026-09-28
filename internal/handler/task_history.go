package handler

import (
	"encoding/json"
	"errors"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func registerTaskHistoryRoutes(authed *gin.RouterGroup, svc *service.Container) {
	group := authed.Group("/tasks", middleware.AdminRequired())
	group.GET("/definitions", func(c *gin.Context) { c.JSON(200, svc.TaskDefinitions()) })
	group.POST("/definitions/:id/run", schedulerRunTaskHandler(svc))
	group.GET("/history", taskHistoryHandler(svc))
	group.GET("/logs", taskLogsHandler(svc, false))
	group.GET("/log-days", taskLogsHandler(svc, true))
	group.GET("/scrape-pending", func(c *gin.Context) {
		page, size, err := operationalPage(c)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		items, total, err := svc.Repo.PendingScrape(c.Request.Context(), c.Query("library_id"), page, size)
		if err != nil {
			c.JSON(500, gin.H{"error": "待刮削列表读取失败"})
			return
		}
		c.JSON(200, gin.H{"items": items, "total": total, "page": page, "page_size": size})
	})
}

func taskHistoryFilter(c *gin.Context) (repository.TaskHistoryFilter, error) {
	page, size, err := operationalPage(c)
	if err != nil {
		return repository.TaskHistoryFilter{}, err
	}
	from, to, err := operationalRange(c, 30)
	if err != nil {
		return repository.TaskHistoryFilter{}, err
	}
	status := c.Query("status")
	if status != "" && status != "running" && status != "completed" && status != "failed" && status != "canceled" {
		return repository.TaskHistoryFilter{}, errors.New("无效任务状态")
	}
	return repository.TaskHistoryFilter{From: from, To: to, Kind: c.Query("kind"), TaskID: c.Query("task_id"), Status: status, Page: page, PageSize: size}, nil
}

func taskHistoryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		f, err := taskHistoryFilter(c)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		records, total, err := svc.Repo.TaskExecutions(c.Request.Context(), f)
		if err != nil {
			c.JSON(500, gin.H{"error": "任务历史读取失败"})
			return
		}
		items := []service.BackgroundTask{}
		for _, record := range records {
			var task service.BackgroundTask
			if json.Unmarshal([]byte(record.Snapshot), &task) == nil {
				task.Details = nil
				items = append(items, task)
			}
		}
		c.JSON(200, gin.H{"items": items, "total": total, "page": f.Page, "page_size": f.PageSize})
	}
}

func taskLogsHandler(svc *service.Container, days bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		f, err := taskHistoryFilter(c)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		if days {
			items, err := svc.Repo.TaskLogDays(c.Request.Context(), f)
			if err != nil {
				c.JSON(500, gin.H{"error": "日志日期读取失败"})
				return
			}
			c.JSON(200, items)
			return
		}
		items, total, err := svc.Repo.TaskLogs(c.Request.Context(), f)
		if err != nil {
			c.JSON(500, gin.H{"error": "任务日志读取失败"})
			return
		}
		c.JSON(200, gin.H{"items": items, "total": total, "page": f.Page, "page_size": f.PageSize})
	}
}
