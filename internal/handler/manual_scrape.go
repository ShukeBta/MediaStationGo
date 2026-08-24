package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type manualScrapeApplyReq struct {
	MediaIDs []string                    `json:"media_ids"`
	Match    service.ManualScrapeRequest `json:"match"`
}

const manualScrapeApplyTimeout = 5 * time.Minute

func manualScrapeSearchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		m, err := svc.Repo.Media.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil || m == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
			return
		}
		results, err := svc.Scraper.ManualSearch(
			c.Request.Context(),
			m,
			c.Query("query"),
			c.DefaultQuery("provider", "all"),
			c.Query("media_type"),
		)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": results})
	}
}

func manualScrapeApplyOneHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req service.ManualScrapeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		applyCtx, cancel := manualScrapeApplyContext(c)
		defer cancel()
		mediaID := c.Param("id")
		media, err := svc.Scraper.ApplyManualMatch(applyCtx, mediaID, req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		reclassifyMediaAfterScrapeWithTypeHints(applyCtx, svc, map[string]string{mediaID: req.MediaType}, mediaID)
		if refreshed, _ := svc.Repo.Media.FindByID(applyCtx, mediaID); refreshed != nil {
			media = refreshed
		}
		c.JSON(http.StatusOK, media)
	}
}

func manualScrapeApplyBatchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req manualScrapeApplyReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ids := compactManualScrapeIDs(req.MediaIDs)
		if len(ids) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "media_ids required"})
			return
		}
		applyCtx, cancel := manualScrapeApplyContext(c)
		defer cancel()
		result, err := svc.Scraper.ApplyManualMatchBatchWithOptions(applyCtx, ids, req.Match, service.ScrapeOptions{})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if len(result.AppliedIDs) > 0 {
			mediaTypeHints := make(map[string]string, len(result.AppliedIDs))
			for _, id := range result.AppliedIDs {
				mediaTypeHints[id] = req.Match.MediaType
			}
			reclassifyMediaAfterScrapeWithTypeHints(applyCtx, svc, mediaTypeHints, result.AppliedIDs...)
		}
		errorsOut := make([]string, 0, len(result.Errors))
		for _, applyErr := range result.Errors {
			errorsOut = append(errorsOut, applyErr.MediaID+": "+applyErr.Err.Error())
		}
		if len(result.AppliedIDs) == 0 && len(errorsOut) > 0 {
			c.JSON(http.StatusInternalServerError, gin.H{"error": strings.Join(errorsOut, "\n")})
			return
		}
		c.JSON(http.StatusOK, gin.H{"applied": len(result.AppliedIDs), "errors": errorsOut})
	}
}

func manualScrapeApplyContext(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(c.Request.Context()), manualScrapeApplyTimeout)
}

func compactManualScrapeIDs(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
