// Package handler — RSS subscription endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type subscriptionReq struct {
	Name          string `json:"name" binding:"required"`
	FeedURL       string `json:"feed_url" binding:"required"`
	Filter        string `json:"filter"`
	MediaType     string `json:"media_type"`
	MediaCategory string `json:"media_category"`
	SavePath      string `json:"save_path"`
	SearchMode    string `json:"search_mode"`
	IMDBID        string `json:"imdb_id"`
	Source        string `json:"source"`
	PosterURL     string `json:"poster_url"`
	BackdropURL   string `json:"backdrop_url"`
	Overview      string `json:"overview"`
	Resolution    string `json:"resolution"`
	Quality       string `json:"quality"`
	Effects       string `json:"effects"`
	ReleaseGroups string `json:"release_groups"`
	ExcludeWords  string `json:"exclude_words"`
	WashEnabled   bool   `json:"wash_enabled"`
	WashPriority  string `json:"wash_priority"`
	TotalEpisodes int    `json:"total_episodes"`
	Priority      int    `json:"priority"`
	Enabled       *bool  `json:"enabled"`
}

func createSubscriptionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req subscriptionReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		s := &model.Subscription{
			UserID:        uid.(string),
			Name:          req.Name,
			FeedURL:       req.FeedURL,
			Filter:        req.Filter,
			MediaType:     req.MediaType,
			MediaCategory: req.MediaCategory,
			SavePath:      req.SavePath,
			SearchMode:    req.SearchMode,
			IMDBID:        req.IMDBID,
			Source:        req.Source,
			PosterURL:     req.PosterURL,
			BackdropURL:   req.BackdropURL,
			Overview:      req.Overview,
			Resolution:    req.Resolution,
			Quality:       req.Quality,
			Effects:       req.Effects,
			ReleaseGroups: req.ReleaseGroups,
			ExcludeWords:  req.ExcludeWords,
			WashEnabled:   req.WashEnabled,
			WashPriority:  req.WashPriority,
			TotalEpisodes: req.TotalEpisodes,
			Priority:      req.Priority,
			Enabled:       enabled,
		}
		enrichSubscriptionArtwork(c.Request.Context(), svc, s)
		if err := svc.Subscription.Create(c.Request.Context(), s); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		enriched := []model.Subscription{*s}
		service.EnrichSubscriptionProgress(c.Request.Context(), svc.Repo, enriched)
		*s = enriched[0]
		c.JSON(http.StatusCreated, s)
	}
}

func listSubscriptionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Subscription.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		enrichAndPersistSubscriptions(c.Request.Context(), svc, items)
		service.EnrichSubscriptionProgress(c.Request.Context(), svc.Repo, items)
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func deleteSubscriptionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Subscription.Delete(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func runSubscriptionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Subscription.RunNow(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"queued": n})
	}
}
