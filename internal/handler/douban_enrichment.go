package handler

import (
	"errors"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"net/http"
)

func enrichMediaDoubanHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !requireTasksReady(c, svc) {
			return
		}
		task := svc.Tasks.Start(service.TaskKindDouban, "豆瓣信息补齐", service.TaskUpdate{Stage: "running", Message: "媒体 " + c.Param("id")})
		media, err := svc.Scraper.EnrichFromDouban(c.Request.Context(), c.Param("id"))
		task.Finish(err, service.TaskUpdate{Stage: "finished", Message: "豆瓣信息补齐结束"})
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, service.ErrDoubanSubjectNotFound) {
				status = http.StatusNotFound
			}
			if errors.Is(err, service.ErrDoubanTemporarilyUnavailable) {
				status = http.StatusBadGateway
			}
			if errors.Is(err, service.ErrDoubanBindingChanged) {
				status = http.StatusConflict
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, media)
	}
}
