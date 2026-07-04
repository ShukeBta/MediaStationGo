// Package handler — subscription update + per-subscription site search.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type subscriptionPatchReq struct {
	Name          *string  `json:"name"`
	FeedURL       *string  `json:"feed_url"`
	Filter        *string  `json:"filter"`
	MediaType     *string  `json:"media_type"`
	MediaCategory *string  `json:"media_category"`
	SavePath      *string  `json:"save_path"`
	SearchMode    *string  `json:"search_mode"`
	IMDBID        *string  `json:"imdb_id"`
	Source        *string  `json:"source"`
	PosterURL     *string  `json:"poster_url"`
	BackdropURL   *string  `json:"backdrop_url"`
	Overview      *string  `json:"overview"`
	OriginalName  *string  `json:"original_name"`
	Year          *int     `json:"year"`
	Rating        *float32 `json:"rating"`
	Genres        *string  `json:"genres"`
	Resolution    *string  `json:"resolution"`
	Quality       *string  `json:"quality"`
	Effects       *string  `json:"effects"`
	ReleaseGroups *string  `json:"release_groups"`
	ExcludeWords  *string  `json:"exclude_words"`
	MinSeeders    *int     `json:"min_seeders"`
	MaxSeeders    *int     `json:"max_seeders"`
	MinSizeGB     *float64 `json:"min_size_gb"`
	MaxSizeGB     *float64 `json:"max_size_gb"`
	FreeOnly      *bool    `json:"free_only"`
	WashEnabled   *bool    `json:"wash_enabled"`
	WashPriority  *string  `json:"wash_priority"`
	TotalEpisodes *int     `json:"total_episodes"`
	Priority      *int     `json:"priority"`
	Enabled       *bool    `json:"enabled"`
}

// updateSubscriptionHandler patches a subscription row.
func updateSubscriptionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var patch subscriptionPatchReq
		if err := c.ShouldBindJSON(&patch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates := subscriptionPatchUpdates(patch)
		if len(updates) == 0 {
			c.Status(http.StatusNoContent)
			return
		}
		if err := svc.Repo.DB.WithContext(c.Request.Context()).
			Model(&model.Subscription{}).
			Where("id = ?", c.Param("id")).
			Updates(updates).Error; err != nil {
			logSubscriptionWarn(svc, "subscription update failed",
				zap.String("user_id", subscriptionRequestUserID(c)),
				zap.String("subscription_id", c.Param("id")),
				zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		logSubscriptionInfo(svc, "subscription updated",
			zap.String("user_id", subscriptionRequestUserID(c)),
			zap.String("subscription_id", c.Param("id")),
			zap.Strings("fields", subscriptionUpdateFieldNames(updates)))
		c.Status(http.StatusNoContent)
	}
}

func subscriptionPatchUpdates(patch subscriptionPatchReq) map[string]any {
	updates := map[string]any{}
	if patch.Name != nil {
		updates["name"] = *patch.Name
	}
	if patch.FeedURL != nil {
		updates["feed_url"] = *patch.FeedURL
	}
	if patch.Filter != nil {
		updates["filter"] = *patch.Filter
	}
	if patch.MediaType != nil {
		updates["media_type"] = *patch.MediaType
	}
	if patch.MediaCategory != nil {
		updates["media_category"] = *patch.MediaCategory
	}
	if patch.SavePath != nil {
		updates["save_path"] = *patch.SavePath
	}
	if patch.SearchMode != nil {
		updates["search_mode"] = *patch.SearchMode
	}
	if patch.IMDBID != nil {
		updates["imdb_id"] = *patch.IMDBID
	}
	if patch.Source != nil {
		updates["source"] = *patch.Source
	}
	if patch.PosterURL != nil {
		updates["poster_url"] = *patch.PosterURL
	}
	if patch.BackdropURL != nil {
		updates["backdrop_url"] = *patch.BackdropURL
	}
	if patch.Overview != nil {
		updates["overview"] = *patch.Overview
	}
	if patch.OriginalName != nil {
		updates["original_name"] = *patch.OriginalName
	}
	if patch.Year != nil {
		updates["year"] = *patch.Year
	}
	if patch.Rating != nil {
		updates["rating"] = *patch.Rating
	}
	if patch.Genres != nil {
		updates["genres"] = *patch.Genres
	}
	if patch.Resolution != nil {
		updates["resolution"] = *patch.Resolution
	}
	if patch.Quality != nil {
		updates["quality"] = *patch.Quality
	}
	if patch.Effects != nil {
		updates["effects"] = *patch.Effects
	}
	if patch.ReleaseGroups != nil {
		updates["release_groups"] = *patch.ReleaseGroups
	}
	if patch.ExcludeWords != nil {
		updates["exclude_words"] = *patch.ExcludeWords
	}
	if patch.MinSeeders != nil {
		updates["min_seeders"] = *patch.MinSeeders
	}
	if patch.MaxSeeders != nil {
		updates["max_seeders"] = *patch.MaxSeeders
	}
	if patch.MinSizeGB != nil {
		updates["min_size_gb"] = *patch.MinSizeGB
	}
	if patch.MaxSizeGB != nil {
		updates["max_size_gb"] = *patch.MaxSizeGB
	}
	if patch.FreeOnly != nil {
		updates["free_only"] = *patch.FreeOnly
	}
	if patch.WashEnabled != nil {
		updates["wash_enabled"] = *patch.WashEnabled
	}
	if patch.WashPriority != nil {
		updates["wash_priority"] = *patch.WashPriority
	}
	if patch.TotalEpisodes != nil {
		updates["total_episodes"] = *patch.TotalEpisodes
	}
	if patch.Priority != nil {
		updates["priority"] = *patch.Priority
	}
	if patch.Enabled != nil {
		updates["enabled"] = *patch.Enabled
	}
	return updates
}

func subscriptionUpdateFieldNames(updates map[string]any) []string {
	names := make([]string, 0, len(updates))
	for name := range updates {
		names = append(names, name)
	}
	return names
}

// searchSubscriptionHandler runs a one-off keyword search against the
// configured tracker sites for the given subscription. We treat the
// subscription's filter as the search term; this lets the UI preview
// what would be queued without actually downloading anything.
func searchSubscriptionHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var sub model.Subscription
		err := svc.Repo.DB.WithContext(c.Request.Context()).
			Where("id = ?", c.Param("id")).First(&sub).Error
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
			return
		}
		keyword := sub.Filter
		if keyword == "" {
			keyword = sub.Name
		}
		results, err := svc.Site.Search(c.Request.Context(), keyword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": results, "subscription": sub})
	}
}
