// Package handler — alias endpoints used by the Vue UI's media detail
// page that map onto the existing /favourites surface.
//
//	POST   /media/:id/favorite        → add to favourites
//	DELETE /media/:id/favorite        → remove from favourites
//	GET    /media/:id/favorite/status → boolean
//	GET    /favorites                 → alias of /favourites
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// addMediaFavoriteHandler ensures the (user, media) row exists. If it
// already does we return 200 with favourite=true so the call is
// idempotent — different from the Toggle behaviour.
func addMediaFavoriteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		// Check current state.
		var existing model.Favorite
		err := svc.Repo.DB.WithContext(c.Request.Context()).
			Where("user_id = ? AND media_id = ?", uid, c.Param("id")).
			First(&existing).Error
		if err == nil {
			c.JSON(http.StatusOK, gin.H{"favourite": true})
			return
		}
		// Otherwise create.
		fav := &model.Favorite{UserID: toString(uid), MediaID: c.Param("id")}
		if err := svc.Repo.DB.WithContext(c.Request.Context()).Create(fav).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"favourite": true})
	}
}

// removeMediaFavoriteHandler is the idempotent inverse.
func removeMediaFavoriteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		if err := svc.Repo.DB.WithContext(c.Request.Context()).
			Where("user_id = ? AND media_id = ?", uid, c.Param("id")).
			Delete(&model.Favorite{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"favourite": false})
	}
}

// getMediaFavoriteStatusHandler returns the current state.
func getMediaFavoriteStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		var n int64
		_ = svc.Repo.DB.WithContext(c.Request.Context()).
			Model(&model.Favorite{}).
			Where("user_id = ? AND media_id = ?", uid, c.Param("id")).
			Count(&n).Error
		c.JSON(http.StatusOK, gin.H{"favourite": n > 0})
	}
}

// listFavoritesAliasHandler is the /favorites alias of /favourites.
// We reuse the existing service method.
func listFavoritesAliasHandler(svc *service.Container) gin.HandlerFunc {
	return listFavouritesHandler(svc)
}

// aiScrapeMediaHandler asks the scraper to enrich one media row using
// AI-assisted matching. Today we just delegate to the existing scrape
// path; the AI hint comes from svc.AI when configured.
func aiScrapeMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		options, err := scrapeOptionsFromRequest(c, false)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scrape options"})
			return
		}
		m, err := svc.Repo.Media.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil || m == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
			return
		}
		if err := svc.Scraper.EnrichOneWithOptions(c.Request.Context(), m, options); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		reclassifyMediaAfterScrape(c.Request.Context(), svc, m.ID)
		refreshed, _ := svc.Repo.Media.FindByID(c.Request.Context(), m.ID)
		c.JSON(http.StatusOK, refreshed)
	}
}

// scrapeTestHandler validates a (provider, code) pair without touching
// the database.  Useful for the "preview" workflow in the Vue UI.
type scrapeTestReq struct {
	Code string `json:"code" binding:"required"`
}

func scrapeTestHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req scrapeTestReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Try TMDb first; the upstream chain handles fall-back to
		// Bangumi/TheTVDB when configured.
		match, err := svc.TMDb.SearchMovie(c.Request.Context(), req.Code, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"match": match})
	}
}

// organizeBulkHandler triggers organisation across every library when
// the caller hits POST /media/organize without a media id. It mirrors
// the upstream Vue surface.
func organizeBulkHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libs, err := svc.Repo.Library.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]any, 0, len(libs))
		for _, l := range libs {
			resp, err := organizePipeline(svc).Run(c.Request.Context(), service.OrganizePipelineRequest{
				Scope:     service.OrganizeScopeLibrary,
				Trigger:   service.OrganizeTriggerManual,
				TaskName:  "批量整理媒体库：" + l.Name,
				LibraryID: l.ID,
			})
			if err != nil {
				out = append(out, gin.H{"library": l.Name, "error": err.Error()})
				continue
			}
			out = append(out, gin.H{"library": l.Name, "result": resp.Result})
		}
		c.JSON(http.StatusOK, gin.H{"results": out})
	}
}
