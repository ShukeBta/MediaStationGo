// Package handler — auxiliary media endpoints used by the home page
// rails (recent additions) and the admin dashboard summary card.
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func recentMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
		if limit <= 0 || limit > 100 {
			limit = 12
		}
		items, err := svc.Media.ListRecentSeriesCards(c.Request.Context(), limit, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, items)
	}
}

func mediaStatsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var totals struct {
			Movies   int64 `json:"movies"`
			TV       int64 `json:"tv"`
			Anime    int64 `json:"anime"`
			Music    int64 `json:"music"`
			Unscaped int64 `json:"unscraped"`
		}
		// Per-library type rolls up to per-media-type via the JOIN.
		_ = svc.Repo.DB.Model(&model.Media{}).
			Joins("JOIN libraries ON libraries.id = media.library_id").
			Where("libraries.type = ?", "movie").Count(&totals.Movies).Error
		_ = svc.Repo.DB.Model(&model.Media{}).
			Joins("JOIN libraries ON libraries.id = media.library_id").
			Where("libraries.type = ?", "tv").Count(&totals.TV).Error
		_ = svc.Repo.DB.Model(&model.Media{}).
			Joins("JOIN libraries ON libraries.id = media.library_id").
			Where("libraries.type = ?", "anime").Count(&totals.Anime).Error
		_ = svc.Repo.DB.Model(&model.Media{}).
			Joins("JOIN libraries ON libraries.id = media.library_id").
			Where("libraries.type = ?", "music").Count(&totals.Music).Error
		_ = svc.Repo.DB.Model(&model.Media{}).
			Where("scrape_status IS NULL OR scrape_status = '' OR scrape_status = 'pending'").
			Count(&totals.Unscaped).Error

		var totalCount, totalSize, totalSeconds int64
		_ = svc.Repo.DB.Model(&model.Media{}).Count(&totalCount).Error
		_ = svc.Repo.DB.Model(&model.Media{}).
			Select("COALESCE(SUM(size_bytes),0)").Row().Scan(&totalSize)
		_ = svc.Repo.DB.Model(&model.Media{}).
			Select("COALESCE(SUM(duration_sec),0)").Row().Scan(&totalSeconds)

		c.JSON(http.StatusOK, gin.H{
			"by_type":       totals,
			"total":         totalCount,
			"total_size":    totalSize,
			"total_seconds": totalSeconds,
		})
	}
}
