// Package handler — unified search surface that mirrors the Python
// project's /api/search* endpoints. Internally we delegate to the
// existing media + site adapters; advanced/tmdb/sites variants exist
// so the upstream Vue UI's queries don't need rewriting.
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// searchUnifiedHandler is the basic /api/search endpoint.
func searchUnifiedHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Query("q")
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
		if limit <= 0 || limit > 200 {
			limit = 30
		}
		items, err := svc.Media.SearchMediaVisible(c.Request.Context(), q, limit, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": mediaSliceForResponse(c, items), "total": len(items)})
	}
}

// searchAdvancedHandler accepts query + optional filters
// (year, type, library_id) — currently it ignores the filters in the
// SQL but threads them through to the response so the UI can echo
// them back. This keeps API parity without a giant query builder.
func searchAdvancedHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Query("q")
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "30"))
		if limit <= 0 || limit > 200 {
			limit = 30
		}
		items, err := svc.Media.SearchMediaVisible(c.Request.Context(), q, limit, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items": mediaSliceForResponse(c, items),
			"filters": gin.H{
				"year":       c.Query("year"),
				"type":       c.Query("type"),
				"library_id": c.Query("library_id"),
			},
		})
	}
}

// searchTMDbHandler proxies the TMDb /search endpoint via the existing
// SearchMovie helper. Movies and TV use different URLs upstream but
// only the movie path is wired today; TV is best-effort.
func searchTMDbHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Query("query")
		if q == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "query required"})
			return
		}
		if svc.TMDb == nil || !svc.TMDb.Enabled() {
			c.JSON(http.StatusOK, gin.H{"items": []any{}, "note": "tmdb disabled"})
			return
		}
		match, err := svc.TMDb.SearchMovie(c.Request.Context(), q, 0)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out := make([]any, 0, 1)
		if match != nil {
			out = append(out, match)
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
	}
}

// searchSitesHandler mirrors the existing /sites/search but at the
// /search/sites alias the Vue UI uses.
func searchSitesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyword := c.Query("keyword")
		if keyword == "" {
			keyword = c.Query("q")
		}
		if keyword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "keyword required"})
			return
		}
		results, err := svc.Site.Search(c.Request.Context(), keyword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": results})
	}
}
