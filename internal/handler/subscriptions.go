// Package handler — RSS subscription endpoints.
package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type subscriptionReq struct {
	Name          string  `json:"name" binding:"required"`
	FeedURL       string  `json:"feed_url" binding:"required"`
	Filter        string  `json:"filter"`
	MediaType     string  `json:"media_type"`
	MediaCategory string  `json:"media_category"`
	SavePath      string  `json:"save_path"`
	SearchMode    string  `json:"search_mode"`
	IMDBID        string  `json:"imdb_id"`
	Source        string  `json:"source"`
	PosterURL     string  `json:"poster_url"`
	BackdropURL   string  `json:"backdrop_url"`
	Overview      string  `json:"overview"`
	OriginalName  string  `json:"original_name"`
	Year          int     `json:"year"`
	Resolution    string  `json:"resolution"`
	Quality       string  `json:"quality"`
	Effects       string  `json:"effects"`
	ReleaseGroups string  `json:"release_groups"`
	ExcludeWords  string  `json:"exclude_words"`
	MinSeeders    int     `json:"min_seeders"`
	MaxSeeders    int     `json:"max_seeders"`
	MinSizeGB     float64 `json:"min_size_gb"`
	MaxSizeGB     float64 `json:"max_size_gb"`
	FreeOnly      bool    `json:"free_only"`
	WashEnabled   bool    `json:"wash_enabled"`
	WashPriority  string  `json:"wash_priority"`
	TotalEpisodes int     `json:"total_episodes"`
	Priority      int     `json:"priority"`
	Enabled       *bool   `json:"enabled"`
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
			OriginalName:  req.OriginalName,
			Year:          req.Year,
			Resolution:    req.Resolution,
			Quality:       req.Quality,
			Effects:       req.Effects,
			ReleaseGroups: req.ReleaseGroups,
			ExcludeWords:  req.ExcludeWords,
			MinSeeders:    req.MinSeeders,
			MaxSeeders:    req.MaxSeeders,
			MinSizeGB:     req.MinSizeGB,
			MaxSizeGB:     req.MaxSizeGB,
			FreeOnly:      req.FreeOnly,
			WashEnabled:   req.WashEnabled,
			WashPriority:  req.WashPriority,
			TotalEpisodes: req.TotalEpisodes,
			Priority:      req.Priority,
			Enabled:       enabled,
		}
		enrichSubscriptionArtwork(c.Request.Context(), svc, s)
		if err := svc.Subscription.Create(c.Request.Context(), s); err != nil {
			logSubscriptionWarn(svc, "subscription create failed",
				zap.String("user_id", s.UserID),
				zap.String("name", req.Name),
				zap.String("feed_kind", subscriptionFeedKind(req.FeedURL)),
				zap.Bool("enabled", enabled),
				zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		logSubscriptionInfo(svc, "subscription created",
			zap.String("user_id", s.UserID),
			zap.String("subscription_id", s.ID),
			zap.String("name", s.Name),
			zap.String("feed_kind", subscriptionFeedKind(s.FeedURL)),
			zap.String("media_type", s.MediaType),
			zap.String("media_category", s.MediaCategory),
			zap.Bool("enabled", s.Enabled),
			zap.Bool("wash_enabled", s.WashEnabled))
		enriched := []model.Subscription{*s}
		svc.Subscription.EnrichManagementProgress(c.Request.Context(), enriched)
		*s = enriched[0]
		c.JSON(http.StatusCreated, s)
	}
}

func listSubscriptionsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Subscription.List(c.Request.Context())
		if err != nil {
			logSubscriptionWarn(svc, "subscription list failed",
				zap.String("user_id", subscriptionRequestUserID(c)),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		svc.Subscription.EnrichManagementProgress(c.Request.Context(), items)
		go enrichAndPersistSubscriptions(context.Background(), svc, append([]model.Subscription(nil), items...))
		logSubscriptionInfo(svc, "subscription list returned",
			zap.String("user_id", subscriptionRequestUserID(c)),
			zap.Int("count", len(items)),
			zap.Bool("history", false))
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func listSubscriptionHistoryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := svc.Subscription.History(c.Request.Context())
		if err != nil {
			logSubscriptionWarn(svc, "subscription history list failed",
				zap.String("user_id", subscriptionRequestUserID(c)),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		svc.Subscription.EnrichManagementProgress(c.Request.Context(), items)
		logSubscriptionInfo(svc, "subscription list returned",
			zap.String("user_id", subscriptionRequestUserID(c)),
			zap.Int("count", len(items)),
			zap.Bool("history", true))
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func deleteSubscriptionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Subscription.Delete(c.Request.Context(), c.Param("id")); err != nil {
			logSubscriptionWarn(svc, "subscription delete failed",
				zap.String("user_id", subscriptionRequestUserID(c)),
				zap.String("subscription_id", c.Param("id")),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		logSubscriptionInfo(svc, "subscription deleted",
			zap.String("user_id", subscriptionRequestUserID(c)),
			zap.String("subscription_id", c.Param("id")))
		c.Status(http.StatusNoContent)
	}
}

func runSubscriptionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		n, err := svc.Subscription.RunNow(c.Request.Context(), c.Param("id"))
		if err != nil {
			logSubscriptionWarn(svc, "subscription run now failed",
				zap.String("user_id", subscriptionRequestUserID(c)),
				zap.String("subscription_id", c.Param("id")),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		logSubscriptionInfo(svc, "subscription run now completed",
			zap.String("user_id", subscriptionRequestUserID(c)),
			zap.String("subscription_id", c.Param("id")),
			zap.Int("queued", n))
		c.JSON(http.StatusOK, gin.H{"queued": n})
	}
}

func restoreSubscriptionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		sub, err := svc.Subscription.Restore(c.Request.Context(), c.Param("id"))
		if err != nil {
			logSubscriptionWarn(svc, "subscription restore failed",
				zap.String("user_id", subscriptionRequestUserID(c)),
				zap.String("subscription_id", c.Param("id")),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		logSubscriptionInfo(svc, "subscription restored",
			zap.String("user_id", subscriptionRequestUserID(c)),
			zap.String("subscription_id", sub.ID),
			zap.String("name", sub.Name))
		enriched := []model.Subscription{*sub}
		svc.Subscription.EnrichManagementProgress(c.Request.Context(), enriched)
		c.JSON(http.StatusOK, enriched[0])
	}
}
