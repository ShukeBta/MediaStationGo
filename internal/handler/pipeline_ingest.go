package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerAuthedPipelineIngestRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.POST("/pipeline/ingest", middleware.AdminRequired(), pipelineIngestStartHandler(svc))
	authed.GET("/pipeline/ingest/:id", middleware.AdminRequired(), pipelineIngestGetHandler(svc))
}

func pipelineIngestStartHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineIngest == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline ingest service unavailable")
			return
		}
		var req service.PipelineIngestRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		job, err := svc.PipelineIngest.Start(c.Request.Context(), req)
		if err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
			return
		}
		Success(c, job)
	}
}

func pipelineIngestGetHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineIngest == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline ingest service unavailable")
			return
		}
		job, err := svc.PipelineIngest.Get(c.Param("id"))
		if err != nil {
			Error(c, http.StatusNotFound, ErrNotFound, err.Error())
			return
		}
		Success(c, job)
	}
}
