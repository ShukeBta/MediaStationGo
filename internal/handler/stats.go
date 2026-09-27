// Package handler — stats / dashboard endpoints.
package handler

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func statsHandler(svc *service.Container) gin.HandlerFunc {
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
		snap.RecentlyAdded = mediaSliceForResponse(c, snap.RecentlyAdded)
		c.JSON(http.StatusOK, snap)
	}
}

func applyStatsVisibility(c *gin.Context, svc *service.Container, snap *service.Snapshot) error {
	visibility := mediaVisibilityForRequest(c, svc)
	libs, err := svc.Repo.Library.List(c.Request.Context())
	if err != nil {
		return err
	}
	libs = service.FilterDisplayCloudLibraries(c.Request.Context(), svc.Repo, libs)
	var visibleLibraries int64
	activeLibraryIDs := make([]string, 0, len(libs))
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		activeLibraryIDs = append(activeLibraryIDs, lib.ID)
		if service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, lib, visibility) {
			visibleLibraries++
		}
	}
	snap.Libraries = visibleLibraries
	cacheKey := visibleStatsCacheKey(visibility, activeLibraryIDs)
	if svc.Cache != nil {
		var cached visibleStatsCacheValue
		if svc.Cache.GetJSON(c.Request.Context(), cacheKey, &cached) {
			snap.Libraries = cached.Libraries
			snap.MediaCount = cached.MediaCount
			snap.TotalSizeBytes = cached.TotalSizeBytes
			snap.TotalSeconds = cached.TotalSeconds
			snap.RecentlyAdded = cached.RecentlyAdded
			return nil
		}
	}

	q := applyMediaVisibilityQuery(svc.Repo.DB.WithContext(c.Request.Context()).Model(&model.Media{}), visibility)
	q = applyActiveLibraryQuery(q, activeLibraryIDs)
	if err := q.Count(&snap.MediaCount).Error; err != nil {
		return err
	}
	type sumRow struct {
		Size    int64
		Seconds int64
	}
	var sum sumRow
	if err := applyActiveLibraryQuery(applyMediaVisibilityQuery(svc.Repo.DB.WithContext(c.Request.Context()).Model(&model.Media{}), visibility), activeLibraryIDs).
		Select("COALESCE(SUM(size_bytes),0) as size, COALESCE(SUM(duration_sec),0) as seconds").
		Scan(&sum).Error; err != nil {
		return err
	}
	snap.TotalSizeBytes = sum.Size
	snap.TotalSeconds = sum.Seconds

	var recent []model.Media
	if err := applyActiveLibraryQuery(applyMediaVisibilityQuery(svc.Repo.DB.WithContext(c.Request.Context()).Model(&model.Media{}), visibility), activeLibraryIDs).
		Order("created_at desc").
		Limit(12).
		Find(&recent).Error; err != nil {
		return err
	}
	snap.RecentlyAdded = recent
	if svc.Cache != nil {
		svc.Cache.SetJSON(c.Request.Context(), cacheKey, visibleStatsCacheValue{
			Libraries:      snap.Libraries,
			MediaCount:     snap.MediaCount,
			TotalSizeBytes: snap.TotalSizeBytes,
			TotalSeconds:   snap.TotalSeconds,
			RecentlyAdded:  snap.RecentlyAdded,
		}, 10*time.Second)
	}
	return nil
}

type visibleStatsCacheValue struct {
	Libraries      int64         `json:"libraries"`
	MediaCount     int64         `json:"media_count"`
	TotalSizeBytes int64         `json:"total_size_bytes"`
	TotalSeconds   int64         `json:"total_seconds"`
	RecentlyAdded  []model.Media `json:"recently_added"`
}

func visibleStatsCacheKey(visibility service.MediaVisibility, activeLibraryIDs []string) string {
	allowed := append([]string(nil), visibility.AllowedLibraryIDs...)
	hidden := append([]string(nil), visibility.HiddenLibraryIDs...)
	active := append([]string(nil), activeLibraryIDs...)
	sort.Strings(allowed)
	sort.Strings(hidden)
	sort.Strings(active)
	sum := sha1.Sum([]byte(strings.Join([]string{
		"visible",
		strings.Join(active, ","),
		strings.Join(allowed, ","),
		strings.Join(hidden, ","),
		boolString(visibility.IncludeNSFW),
	}, "|")))
	return "stats:visible:" + hex.EncodeToString(sum[:])
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func applyMediaVisibilityQuery(q *gorm.DB, visibility service.MediaVisibility) *gorm.DB {
	if !visibility.IncludeNSFW {
		q = q.Where("nsfw = ?", false)
	}
	if len(visibility.HiddenLibraryIDs) > 0 {
		q = q.Where("library_id NOT IN ?", visibility.HiddenLibraryIDs)
	}
	if len(visibility.AllowedLibraryIDs) > 0 {
		q = q.Where("library_id IN ?", visibility.AllowedLibraryIDs)
	}
	return q
}

func applyActiveLibraryQuery(q *gorm.DB, libraryIDs []string) *gorm.DB {
	if len(libraryIDs) == 0 {
		return q.Where("1 = 0")
	}
	return q.Where("library_id IN ?", libraryIDs)
}
