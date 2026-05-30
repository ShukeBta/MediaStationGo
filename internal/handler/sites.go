// Package handler — site management (PT/BT tracker CRUD + cross-site search).
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// ─── CRUD ────────────────────────────────────────────────────────────────────

func listSitesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		sites, err := svc.Site.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": sites})
	}
}

func getSiteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		site, err := svc.Site.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if site == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, site)
	}
}

type createSiteReq struct {
	Name       string `json:"name" binding:"required"`
	BaseURL    string `json:"base_url" binding:"required"`
	SiteType   string `json:"site_type"`
	AuthType   string `json:"auth_type"`
	Cookie     string `json:"cookie"`
	APIKey     string `json:"api_key"`
	AuthHeader string `json:"auth_header"`
	UserAgent  string `json:"user_agent"`
	RSSURL     string `json:"rss_url"`
	Timeout    int    `json:"timeout"`
	Priority   int    `json:"priority"`
	UseProxy   bool   `json:"use_proxy"`
	Enabled    *bool  `json:"enabled"`
	Downloader string `json:"downloader"`
}

func createSiteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createSiteReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		// Pack fields not in the core model into Extra JSON.
		extraMap := map[string]any{}
		if req.UserAgent != "" {
			extraMap["user_agent"] = req.UserAgent
		}
		if req.RSSURL != "" {
			extraMap["rss_url"] = req.RSSURL
		}
		if req.Timeout > 0 {
			extraMap["timeout"] = req.Timeout
		}
		if req.Priority > 0 {
			extraMap["priority"] = req.Priority
		}
		extraMap["use_proxy"] = req.UseProxy
		if req.Downloader != "" {
			extraMap["downloader"] = req.Downloader
		}
		extraJSON, _ := json.Marshal(extraMap)

		site := &model.Site{
			Name:       req.Name,
			URL:        req.BaseURL,
			Type:       req.SiteType,
			AuthType:   req.AuthType,
			Cookie:     req.Cookie,
			APIKey:     req.APIKey,
			AuthHeader: req.AuthHeader,
			Extra:      string(extraJSON),
			Enabled:    enabled,
		}
		if err := svc.Site.Create(c.Request.Context(), site); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, site)
	}
}

func updateSiteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var patch map[string]any
		if err := c.ShouldBindJSON(&patch); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := svc.Site.Update(c.Request.Context(), c.Param("id"), patch); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Return the updated row.
		site, _ := svc.Site.FindByID(c.Request.Context(), c.Param("id"))
		c.JSON(http.StatusOK, site)
	}
}

func deleteSiteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Site.Delete(c.Request.Context(), c.Param("id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// ─── Connection test ─────────────────────────────────────────────────────────

func testSiteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		ok, msg, err := svc.Site.TestConnection(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"success": ok, "message": msg})
	}
}

// ─── Cross-site search ───────────────────────────────────────────────────────

func siteSearchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyword := c.Query("keyword")
		if keyword == "" {
			keyword = c.Query("q")
		}
		results, err := svc.Site.Search(c.Request.Context(), keyword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": results, "total": len(results)})
	}
}
