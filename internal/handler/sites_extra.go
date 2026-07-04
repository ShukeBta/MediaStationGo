// Package handler — extra site endpoints used by the Vue UI:
//
//	GET /sites/:id/resource → keyword search scoped to one site
//	GET /sites/:id/userdata → cookie-derived user info (stubbed)
package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// siteResourceHandler runs a search restricted to a single site.
//
// We reuse the full SiteService.Search() and post-filter by site_id;
// it's not the hottest path so we trade simplicity for speed here.
func siteResourceHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		keyword := c.Query("keyword")
		if keyword == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "keyword required"})
			return
		}
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		items, err := svc.Site.SearchSite(c.Request.Context(), c.Param("id"), keyword, page)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items, "total": len(items)})
	}
}

// siteUserdataHandler returns whatever the site exposes about the
// authenticated user (upload/download stats, ratio, etc.). This is a
// stub: we report the cookie length so the UI can confirm a login is
// present, but full per-site parsing is out of scope here.
func siteUserdataHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		s, err := svc.Site.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil || s == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "site not found"})
			return
		}
		loginStatus := "unknown"
		if s.LastError == "ok" {
			loginStatus = "ok"
		} else if s.LastError != "" {
			loginStatus = "fail"
		}
		c.JSON(http.StatusOK, gin.H{
			"site_id":      s.ID,
			"name":         s.Name,
			"cookie_set":   len(s.Cookie) > 0,
			"login_status": loginStatus,
			"note":         "userdata parsing not implemented; stub",
		})
	}
}
