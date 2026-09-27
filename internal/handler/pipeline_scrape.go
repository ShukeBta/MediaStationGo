package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerAuthedPipelineScrapeRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.POST("/pipeline/media/:id/scrape", middleware.AdminRequired(), pipelineScrapeMediaHandler(svc))
}

func pipelineScrapeMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineScrape == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline scrape service unavailable")
			return
		}
		var req service.PipelineScrapeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		result, err := svc.PipelineScrape.Scrape(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
			return
		}
		Success(c, result)
	}
}
