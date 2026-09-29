package handler

import (
	"net/http"
	"net/url"

	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

type loginShowcaseItem struct {
	Title      string `json:"title"`
	Overview   string `json:"overview"`
	ArtworkURL string `json:"artwork_url"`
	Year       int    `json:"year"`
}

func loginShowcaseHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		items := make([]loginShowcaseItem, 0, 8)
		if svc == nil || svc.Repo == nil || svc.Repo.Media == nil {
			c.JSON(http.StatusOK, gin.H{"items": items})
			return
		}
		rows, err := svc.Repo.Media.LoginShowcase(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "showcase unavailable"})
			return
		}
		for _, row := range rows {
			items = append(items, loginShowcaseItem{Title: row.Title, Overview: row.Overview, Year: row.Year,
				ArtworkURL: "/api/auth/showcase/artwork/" + url.PathEscape(row.ID)})
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func loginShowcaseArtworkHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if svc == nil || svc.Repo == nil || svc.Repo.Media == nil || svc.ImageProxy == nil {
			c.Status(http.StatusNotFound)
			return
		}
		raw, err := svc.Repo.Media.LoginShowcaseArtwork(c.Request.Context(), c.Param("id"))
		if err != nil || raw == "" {
			c.Status(http.StatusNotFound)
			return
		}
		// Only the persisted artwork may be fetched. Ignore client URLs, refresh
		// flags and variant sizes on this unauthenticated, bounded image route.
		req := c.Request.Clone(c.Request.Context())
		req.URL.RawQuery = ""
		if err := svc.ImageProxy.Serve(req.Context(), c.Writer, req, raw); err != nil {
			c.Status(http.StatusNotFound)
		}
	}
}
