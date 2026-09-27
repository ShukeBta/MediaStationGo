// Package handler — recycle bin endpoints.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type recycleBatchReq struct {
	MediaIDs []string `json:"media_ids"`
}

func deleteMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Media.SoftDeleteBy(c.Request.Context(), c.Param("id"), middleware.GetUserID(c), "media"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func softDeleteMediaBatchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req recycleBatchReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ids := compactManualScrapeIDs(req.MediaIDs)
		if len(ids) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "media_ids required"})
			return
		}
		applied, err := svc.Media.SoftDeleteManyBy(c.Request.Context(), ids, middleware.GetUserID(c), "media")
		if err != nil {
			writeMediaVersionError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"applied": applied})
	}
}

func listRecycleHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Media.ListRecycleBinForUser(
			c.Request.Context(), middleware.GetUserID(c), middleware.IsAdmin(c), 200,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": recycleItemsForResponse(c, items)})
	}
}

func restoreMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Media.RestoreDeletedForUser(c.Request.Context(), c.Param("id"), middleware.GetUserID(c), middleware.IsAdmin(c)); err != nil {
			writeMediaVersionError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func restoreMediaBatchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req recycleBatchReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		action := func(ctx context.Context, id string) error {
			return svc.Media.RestoreDeletedForUser(ctx, id, middleware.GetUserID(c), middleware.IsAdmin(c))
		}
		applied, errorsOut, failedIDs := runRecycleBatch(c, compactManualScrapeIDs(req.MediaIDs), action)
		if applied == 0 && len(errorsOut) > 0 {
			c.JSON(http.StatusInternalServerError, gin.H{"error": strings.Join(errorsOut, "\n"), "failed_ids": failedIDs})
			return
		}
		c.JSON(http.StatusOK, gin.H{"applied": applied, "errors": errorsOut, "failed_ids": failedIDs})
	}
}

func purgeMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Media.PurgeDeletedForUser(c.Request.Context(), c.Param("id"), middleware.GetUserID(c), middleware.IsAdmin(c)); err != nil {
			writeMediaVersionError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func purgeMediaBatchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req recycleBatchReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		action := func(ctx context.Context, id string) error {
			return svc.Media.PurgeDeletedForUser(ctx, id, middleware.GetUserID(c), middleware.IsAdmin(c))
		}
		applied, errorsOut, failedIDs := runRecycleBatch(c, compactManualScrapeIDs(req.MediaIDs), action)
		if applied == 0 && len(errorsOut) > 0 {
			c.JSON(http.StatusInternalServerError, gin.H{"error": strings.Join(errorsOut, "\n"), "failed_ids": failedIDs})
			return
		}
		c.JSON(http.StatusOK, gin.H{"applied": applied, "errors": errorsOut, "failed_ids": failedIDs})
	}
}

func getMediaVersionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		media, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		result, err := svc.Media.ListMediaVersions(
			c.Request.Context(), media.ID, middleware.GetUserID(c), middleware.IsAdmin(c),
		)
		if err != nil {
			writeMediaVersionError(c, err)
			return
		}
		c.JSON(http.StatusOK, mediaVersionListForResponse(c, result))
	}
}

func getMediaPartsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		media, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		result, err := svc.Media.ListMediaParts(c.Request.Context(), media.ID)
		if err != nil {
			writeMediaVersionError(c, err)
			return
		}
		c.JSON(http.StatusOK, mediaPartListForResponse(c, result))
	}
}

func deleteMediaVersionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		media, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		result, err := svc.Media.DeleteMediaVersion(
			c.Request.Context(), media.ID, c.Param("version_id"), middleware.GetUserID(c), middleware.IsAdmin(c),
		)
		if err != nil {
			writeMediaVersionError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func writeMediaVersionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrMediaVersionNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrMediaVersionForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "无权管理该片源版本"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

func runRecycleBatch(c *gin.Context, ids []string, action func(context.Context, string) error) (int, []string, []string) {
	if len(ids) == 0 {
		return 0, []string{"media_ids required"}, nil
	}
	applied := 0
	errorsOut := make([]string, 0)
	failedIDs := make([]string, 0)
	for _, id := range ids {
		if err := action(c.Request.Context(), id); err != nil {
			errorsOut = append(errorsOut, id+": "+err.Error())
			failedIDs = append(failedIDs, id)
			continue
		}
		applied++
	}
	return applied, errorsOut, failedIDs
}
