package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type pipelineDeletedMediaHideCandidatesRequest struct {
	Limit int `json:"limit"`
}

func pipelineDeletedMediaHideCandidatesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline maintenance service unavailable")
			return
		}
		var req pipelineDeletedMediaHideCandidatesRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		result, err := svc.PipelineMaintenance.ListDeletedMediaHideCandidates(c.Request.Context(), req.Limit)
		if err != nil {
			Error(c, http.StatusInternalServerError, ErrInternal, err.Error())
			return
		}
		Success(c, result)
	}
}

func pipelineMigrationSearchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline maintenance service unavailable")
			return
		}
		var req service.PipelineMigrationSearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		result, err := svc.PipelineMaintenance.SearchMigrationCandidates(c.Request.Context(), req)
		if err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
			return
		}
		Success(c, result)
	}
}

func pipelineMigrationValidateHandler(svc *service.Container) gin.HandlerFunc {
	return pipelineMigrationHandler(svc, false)
}

func pipelineMigrationApplyHandler(svc *service.Container) gin.HandlerFunc {
	return pipelineMigrationHandler(svc, true)
}

func pipelineMigrationHandler(svc *service.Container, apply bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline maintenance service unavailable")
			return
		}
		var req service.PipelineMigrationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		var (
			result service.PipelineMigrationResult
			err    error
		)
		if apply {
			result, err = svc.PipelineMaintenance.ApplyMigration(c.Request.Context(), req)
		} else {
			result, err = svc.PipelineMaintenance.ValidateMigration(c.Request.Context(), req)
		}
		if err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, err.Error())
			return
		}
		Success(c, result)
	}
}
