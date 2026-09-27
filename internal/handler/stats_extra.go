// Package handler — richer dashboard statistics endpoints.
//
// /api/stats already returns the basic snapshot. The Vue admin
// dashboard also uses:
//
//	/api/stats/overview      — counts + total size + total seconds
//	/api/stats/trend         — daily play count over last N days
//	/api/stats/top-content   — top played media (by play count)
//	/api/stats/libraries     — per-library item count + size
//	/api/stats/monitor       — live CPU/mem/disk
package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func statsOverviewHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap, err := svc.Stats.Compute(c.Request.Context(), svc.Cfg.App.DataDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if err := applyStatsVisibility(c, svc, snap); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"libraries":     snap.Libraries,
			"media_count":   snap.MediaCount,
			"users_count":   snap.UsersCount,
			"total_size":    snap.TotalSizeBytes,
			"total_seconds": snap.TotalSeconds,
			"generated_at":  snap.GeneratedAt,
		})
	}
}

// statsTrendHandler returns play counts per day for the last N days.
func statsTrendHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		days, _ := strconv.Atoi(c.DefaultQuery("days", "14"))
		if days <= 0 || days > 90 {
			days = 14
		}
		// Use the playback_history table; one row per (user, media)
		// per day if we group by date(watched_at).
		type bucket struct {
			Day   string `json:"day"`
			Count int64  `json:"count"`
		}
		out := make([]bucket, 0, days)
		now := time.Now().UTC()
		for i := days - 1; i >= 0; i-- {
			start := now.AddDate(0, 0, -i).Truncate(24 * time.Hour)
			end := start.Add(24 * time.Hour)
			var n int64
			_ = svc.Repo.DB.Model(&model.PlaybackHistory{}).
				Where("watched_at >= ? AND watched_at < ?", start, end).
				Count(&n).Error
			out = append(out, bucket{
				Day:   start.Format("2006-01-02"),
				Count: n,
			})
		}
		c.JSON(http.StatusOK, gin.H{"trend": out, "days": days})
	}
}

// statsTopContentHandler returns the most-watched media items.
func statsTopContentHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
		if limit <= 0 || limit > 50 {
			limit = 10
		}
		type row struct {
			MediaID    string    `json:"media_id"`
			PlayCount  int64     `json:"play_count"`
			LastPlayed time.Time `json:"last_played"`
		}
		var rows []row
		_ = svc.Repo.DB.Table("playback_histories").
			Select("media_id, COUNT(*) as play_count, MAX(watched_at) as last_played").
			Group("media_id").
			Order("play_count desc").
			Limit(limit).
			Scan(&rows).Error
		// Hydrate media titles in a single query.
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.MediaID)
		}
		mIdx := map[string]model.Media{}
		if len(ids) > 0 {
			var media []model.Media
			_ = svc.Repo.DB.Where("id IN ?", ids).Find(&media).Error
			visibility := mediaVisibilityForRequest(c, svc)
			for _, m := range media {
				if !visibility.Allows(&m) {
					continue
				}
				mIdx[m.ID] = mediaForResponse(c, m)
			}
		}
		out := make([]gin.H, 0, len(rows))
		for _, r := range rows {
			media, ok := mIdx[r.MediaID]
			if !ok {
				continue
			}
			out = append(out, gin.H{
				"media":       media,
				"play_count":  r.PlayCount,
				"last_played": r.LastPlayed,
			})
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
	}
}

// statsLibrariesHandler returns per-library counts + size.
func statsLibrariesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libs, err := svc.Repo.Library.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		libs = service.FilterDisplayCloudLibraries(c.Request.Context(), svc.Repo, libs)
		out := make([]gin.H, 0, len(libs))
		visibility := mediaVisibilityForRequest(c, svc)
		for _, l := range libs {
			if !l.Enabled {
				continue
			}
			if !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, l, visibility) {
				continue
			}
			libraryIDs, err := service.MergedLibraryIDsForLibrary(c.Request.Context(), svc.Repo, l.ID)
			if err != nil || len(libraryIDs) == 0 {
				libraryIDs = []string{l.ID}
			}
			var count int64
			var size int64
			_ = applyMediaVisibilityQuery(svc.Repo.DB.Model(&model.Media{}), visibility).
				Where("library_id IN ?", libraryIDs).
				Count(&count).Error
			_ = applyMediaVisibilityQuery(svc.Repo.DB.Model(&model.Media{}), visibility).
				Where("library_id IN ?", libraryIDs).
				Select("COALESCE(SUM(size_bytes),0)").Row().Scan(&size)
			out = append(out, gin.H{
				"library":    l,
				"item_count": count,
				"total_size": size,
			})
		}
		c.JSON(http.StatusOK, gin.H{"libraries": out})
	}
}

// statsMonitorHandler returns live system resource usage; this is just
// the Hardware portion of the snapshot but with a snappy schema.
func statsMonitorHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap, err := svc.Stats.Compute(c.Request.Context(), svc.Cfg.App.DataDir)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, snap.Hardware)
	}
}
