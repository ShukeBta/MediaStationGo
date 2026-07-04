// Package handler — TV series endpoints.
//
// These return episode lists grouped by season number for a library that
// holds TV episodes. Series rows are distinct from Movies — the front
// end uses /api/libraries/:id/seasons to render a season selector.
package handler

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// seasonGroup is the JSON returned to the React UI per season.
type seasonGroup struct {
	Season   int           `json:"season"`
	Episodes []model.Media `json:"episodes"`
}

func listSeasonsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libID := c.Param("id")
		if lib, err := svc.Repo.Library.FindByID(c.Request.Context(), libID); err == nil && lib != nil {
			if !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, *lib, mediaVisibilityForRequest(c, svc)) {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
		}
		visibility := mediaVisibilityForRequest(c, svc)
		var rows []model.Media
		const pageSize = 2000
		for page := 1; ; page++ {
			pageRows, total, err := svc.Media.ListMediaVisible(c.Request.Context(), libID, page, pageSize, visibility)
			if err != nil && err != gorm.ErrRecordNotFound {
				writeInternalOrCanceled(c, err)
				return
			}
			rows = append(rows, pageRows...)
			if int64(len(rows)) >= total || len(pageRows) < pageSize {
				break
			}
		}
		buckets := make(map[int][]model.Media)
		for _, r := range rows {
			if !visibility.Allows(&r) {
				continue
			}
			buckets[r.SeasonNum] = append(buckets[r.SeasonNum], r)
		}
		out := make([]seasonGroup, 0, len(buckets))
		for s, items := range buckets {
			out = append(out, seasonGroup{Season: s, Episodes: items})
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Season < out[j].Season })
		c.JSON(http.StatusOK, gin.H{"seasons": out})
	}
}

func listLibrarySeriesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libID := c.Param("id")
		if lib, err := svc.Repo.Library.FindByID(c.Request.Context(), libID); err == nil && lib != nil {
			if !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, *lib, mediaVisibilityForRequest(c, svc)) {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
		}
		items, total, err := svc.Media.ListLibrarySeriesCards(c.Request.Context(), libID, mediaVisibilityForRequest(c, svc))
		if err != nil {
			writeInternalOrCanceled(c, err)
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		size, _ := strconv.Atoi(c.DefaultQuery("page_size", "500"))
		if page < 1 {
			page = 1
		}
		if size <= 0 || size > 1000 {
			size = 500
		}
		start := (page - 1) * size
		if start > len(items) {
			start = len(items)
		}
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		pageItems := items[start:end]
		if pageItems == nil {
			// 非 nil 空切片，避免空库返回 "items": null 触发前端崩溃。
			pageItems = []service.SeriesCard{}
		}
		c.JSON(http.StatusOK, gin.H{
			"items":     pageItems,
			"total":     total,
			"page":      page,
			"page_size": size,
		})
	}
}

func listLibrarySeriesEpisodesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libID := c.Param("id")
		key := c.Query("key")
		if key == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "key is required"})
			return
		}
		if lib, err := svc.Repo.Library.FindByID(c.Request.Context(), libID); err == nil && lib != nil {
			if !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, *lib, mediaVisibilityForRequest(c, svc)) {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
				return
			}
		}
		items, err := svc.Media.ListLibrarySeriesEpisodes(c.Request.Context(), libID, key, mediaVisibilityForRequest(c, svc))
		if err != nil {
			writeInternalOrCanceled(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
	}
}
