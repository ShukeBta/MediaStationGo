package handler

import (
	"errors"
	"net/http"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func tmdbSeriesCatalogHandler(svc *service.Container, refresh bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc.TMDbCatalog == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TMDb 目录服务不可用"})
			return
		}
		media, err := svc.Repo.Media.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if media.TMDbID <= 0 || media.EpisodeNum <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该媒体尚未匹配 TMDb 剧集"})
			return
		}
		if refresh {
			id, err := svc.TMDbCatalog.StartSeriesRefresh(svc.Context(), media.TMDbID)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, service.ErrSchedulerJobAlreadyRunning) {
					status = http.StatusConflict
				}
				c.JSON(status, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusAccepted, gin.H{"task_id": id})
			return
		}
		visibility := mediaVisibilityForRequest(c, svc)
		catalog, err := svc.TMDbCatalog.SeriesCatalog(c.Request.Context(), media, func(item *model.Media) bool { return visibility.Allows(item) })
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, catalog)
	}
}
