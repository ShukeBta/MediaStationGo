package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func updateMediaAggregationHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req service.MediaAggregationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := svc.Media.UpdateMediaAggregation(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			status := http.StatusBadRequest
			if errors.Is(err, gorm.ErrRecordNotFound) {
				status = http.StatusNotFound
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		if svc.Audit != nil {
			svc.Audit.Record(c.Request.Context(), toString(uid), "media.aggregation."+result.Action, c.Param("id"), c.ClientIP(), result.GroupKey)
		}
		c.JSON(http.StatusOK, result)
	}
}
