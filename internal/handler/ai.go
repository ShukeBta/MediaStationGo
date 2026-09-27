// Package handler — AI integration endpoints.
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type smartSearchReq struct {
	Query string `json:"query" binding:"required"`
}

func smartSearchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req smartSearchReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		intent, err := svc.AI.SmartSearch(c.Request.Context(), req.Query)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Run the actual library search using the cleaned query so the
		// caller can render local + external results in one round-trip.
		items, err := svc.Media.SearchMediaVisible(c.Request.Context(), intent.Query, 60, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		external := service.SearchExternalMedia(
			c.Request.Context(),
			intent.Query,
			intent.Year,
			intent.Type,
			svc.TMDb,
			svc.Douban,
			svc.Bangumi,
		)
		service.EnrichExternalMediaAvailability(c.Request.Context(), svc.Repo, external)
		c.JSON(http.StatusOK, gin.H{
			"intent":         intent,
			"items":          mediaSliceForResponse(c, items),
			"external_items": external,
		})
	}
}

func aiRecommendHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		hist, err := svc.Playback.RecentHistory(c.Request.Context(), toString(uid), 10)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		titles := make([]string, 0, len(hist))
		visibility := mediaVisibilityForRequest(c, svc)
		for _, h := range hist {
			if h.Media != nil && visibility.Allows(h.Media) && strings.TrimSpace(h.Media.Title) != "" {
				titles = append(titles, h.Media.Title)
			}
		}
		out, err := svc.AI.Recommend(c.Request.Context(), titles, 8)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"titles": out})
	}
}

func aiStatusHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, svc.AI.Status(c.Request.Context()))
	}
}
