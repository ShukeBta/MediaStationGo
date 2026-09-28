package handler

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type mediaMigrationRequest struct {
	TargetCategory string `json:"target_category" binding:"required"`
}

func getMediaMigrationHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusServiceUnavailable, ErrInternal, "media migration service unavailable")
			return
		}
		candidate, err := svc.PipelineMaintenance.MigrationCandidateForMedia(c.Request.Context(), c.Param("id"))
		if err != nil {
			Error(c, mediaMigrationHTTPStatus(err), ErrInvalidParams, err.Error())
			return
		}
		categories := make([]string, 0, 4)
		for _, category := range []string{"movie", "tv", "anime", "adult", "other"} {
			if category != candidate.Category {
				categories = append(categories, category)
			}
		}
		c.JSON(http.StatusOK, gin.H{"candidate": candidate, "target_categories": categories})
	}
}

func validateMediaMigrationHandler(svc *service.Container) gin.HandlerFunc {
	return mediaMigrationActionHandler(svc, false)
}

func applyMediaMigrationHandler(svc *service.Container) gin.HandlerFunc {
	return mediaMigrationActionHandler(svc, true)
}

func mediaMigrationActionHandler(svc *service.Container, apply bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.PipelineMaintenance == nil {
			Error(c, http.StatusServiceUnavailable, ErrInternal, "media migration service unavailable")
			return
		}
		var req mediaMigrationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "target_category is required")
			return
		}
		req.TargetCategory = strings.ToLower(strings.TrimSpace(req.TargetCategory))
		if !validMediaMigrationCategory(req.TargetCategory) {
			Error(c, http.StatusBadRequest, ErrInvalidParams, "invalid target_category")
			return
		}
		var (
			result service.MediaMigrationPreview
			err    error
		)
		if apply {
			result, err = svc.PipelineMaintenance.ApplyMediaMigration(
				c.Request.Context(), middleware.GetUserID(c), c.Param("id"), req.TargetCategory,
			)
		} else {
			result, err = svc.PipelineMaintenance.ValidateMediaMigration(
				c.Request.Context(), middleware.GetUserID(c), c.Param("id"), req.TargetCategory,
			)
		}
		if err != nil {
			Error(c, mediaMigrationHTTPStatus(err), ErrConflict, err.Error())
			return
		}
		if apply && svc.Log != nil {
			svc.Log.Info("admin media migration completed",
				zap.String("user_id", middleware.GetUserID(c)),
				zap.String("media_id", c.Param("id")),
				zap.String("source_openlist_path", result.Result.SourceOpenListPath),
				zap.String("target_openlist_path", result.Result.TargetOpenListPath),
				zap.Int("media_count", result.Result.MediaCount),
			)
		}
		c.JSON(http.StatusOK, result)
	}
}

func validMediaMigrationCategory(category string) bool {
	switch category {
	case "movie", "tv", "anime", "adult", "other":
		return true
	default:
		return false
	}
}

func mediaMigrationHTTPStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	var statusErr interface{ HTTPStatus() int }
	if errors.As(err, &statusErr) {
		status := statusErr.HTTPStatus()
		if status >= 400 && status <= 599 {
			return status
		}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "not found") {
		return http.StatusNotFound
	}
	if strings.Contains(message, "unavailable") {
		return http.StatusServiceUnavailable
	}
	return http.StatusConflict
}
