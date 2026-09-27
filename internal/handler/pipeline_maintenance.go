package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type pipelineMaintenanceRequest struct {
	Category         string   `json:"category"`
	LibraryID        string   `json:"library_id"`
	RootID           string   `json:"root_id"`
	RootOpenListPath string   `json:"root_openlist_path"`
	OpenListPaths    []string `json:"openlist_paths"`
	NewMediaID       string   `json:"new_media_id"`
	NewOpenListPaths []string `json:"new_openlist_paths"`
}

func registerAuthedPipelineMaintenanceRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/pipeline/media/:id/subtitle-status", middleware.AdminRequired(), pipelineSubtitleStatusHandler(svc))
	authed.POST("/pipeline/media/:id/repair-movie-extras", middleware.AdminRequired(), pipelineRepairMovieExtrasHandler(svc))
	authed.POST("/pipeline/media/:id/repair-episode-visibility", middleware.AdminRequired(), pipelineRepairEpisodeVisibilityHandler(svc))
	authed.POST("/pipeline/media/:id/replace-work-source", middleware.AdminRequired(), pipelineReplaceWorkSourceHandler(svc))
	authed.POST("/pipeline/deleted-media/hide-candidates", middleware.AdminRequired(), pipelineDeletedMediaHideCandidatesHandler(svc))
	authed.POST("/pipeline/deleted-media/prune", middleware.AdminRequired(), pipelinePruneDeletedMediaHandler(svc))
	authed.POST("/pipeline/migrations/search", middleware.AdminRequired(), pipelineMigrationSearchHandler(svc))
	authed.POST("/pipeline/migrations/validate", middleware.AdminRequired(), pipelineMigrationValidateHandler(svc))
	authed.POST("/pipeline/migrations/apply", middleware.AdminRequired(), pipelineMigrationApplyHandler(svc))
}

func pipelineSubtitleStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Subtitle == nil || svc.Stream == nil || svc.FFprobe == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "subtitle detection service unavailable")
			return
		}
		result, err := svc.Subtitle.Presence(c.Request.Context(), c.Param("id"), svc.Stream, svc.FFprobe)
		if err != nil {
			Error(c, http.StatusInternalServerError, ErrInternal, err.Error())
			return
		}
		Success(c, result)
	}
}

func pipelineReplaceWorkSourceHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline maintenance service unavailable")
			return
		}
		var req pipelineMaintenanceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		result, err := svc.PipelineMaintenance.ReplaceWorkSource(
			c.Request.Context(), c.Param("id"), req.NewMediaID, pipelineMaintenanceTarget(req), req.NewOpenListPaths,
		)
		if err != nil {
			Error(c, http.StatusConflict, ErrInvalidParams, err.Error())
			return
		}
		Success(c, result)
	}
}

func pipelineRepairMovieExtrasHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline maintenance service unavailable")
			return
		}
		var req pipelineMaintenanceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		result, err := svc.PipelineMaintenance.RepairMovieExtras(c.Request.Context(), c.Param("id"), pipelineMaintenanceTarget(req))
		if err != nil {
			Error(c, http.StatusInternalServerError, ErrInternal, err.Error())
			return
		}
		Success(c, result)
	}
}

func pipelineRepairEpisodeVisibilityHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline maintenance service unavailable")
			return
		}
		var req pipelineMaintenanceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		result, err := svc.PipelineMaintenance.RepairEpisodeVisibility(c.Request.Context(), c.Param("id"), pipelineMaintenanceTarget(req))
		if err != nil {
			Error(c, http.StatusInternalServerError, ErrInternal, err.Error())
			return
		}
		Success(c, result)
	}
}

func pipelinePruneDeletedMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusInternalServerError, ErrInternal, "pipeline maintenance service unavailable")
			return
		}
		var req pipelineMaintenanceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid request body")
			return
		}
		result, err := svc.PipelineMaintenance.PruneDeletedMedia(c.Request.Context(), pipelineMaintenanceTarget(req), req.OpenListPaths)
		if err != nil {
			Error(c, http.StatusInternalServerError, ErrInternal, err.Error())
			return
		}
		Success(c, result)
	}
}

func pipelineMaintenanceTarget(req pipelineMaintenanceRequest) service.PipelineMaintenanceTarget {
	return service.PipelineMaintenanceTarget{
		Category:         req.Category,
		LibraryID:        req.LibraryID,
		RootID:           req.RootID,
		RootOpenListPath: req.RootOpenListPath,
	}
}
