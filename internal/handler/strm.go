// Package handler — STRM (URL-as-file) admin endpoints.
//
// Setting a media row's strm_url makes the stream handler issue a 302
// redirect to that URL instead of opening a local file. This lets the
// operator expose WebDAV / Alist / S3 / HTTP direct links as ordinary
// MediaStationGo entries.
package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type strmReq struct {
	URL string `json:"url" binding:"required"`
}

func setSTRMHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req strmReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		url := strings.TrimSpace(req.URL)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "url must start with http:// or https://"})
			return
		}
		mediaID := c.Param("id")
		m, err := svc.Repo.Media.FindByID(c.Request.Context(), mediaID)
		if err != nil || m == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "media not found"})
			return
		}
		if err := svc.Repo.DB.WithContext(c.Request.Context()).
			Model(&model.Media{}).
			Where("id = ?", mediaID).
			Update("strm_url", url).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"strm_url": url})
	}
}

func clearSTRMHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Repo.DB.WithContext(c.Request.Context()).
			Model(&model.Media{}).
			Where("id = ?", c.Param("id")).
			Update("strm_url", "").Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

// importSTRMHandler creates a media row directly from a (library_id, title, url)
// tuple — useful for adding a streaming-only entry without an on-disk file.
type importSTRMReq struct {
	LibraryID string `json:"library_id" binding:"required"`
	Title     string `json:"title" binding:"required"`
	URL       string `json:"url" binding:"required"`
}

func importSTRMHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req importSTRMReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		url := strings.TrimSpace(req.URL)
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "url must start with http:// or https://"})
			return
		}
		m := &model.Media{
			LibraryID: req.LibraryID,
			Title:     req.Title,
			Path:      url,
			STRMURL:   url,
			Container: "strm",
		}
		if err := svc.Repo.Media.Upsert(c.Request.Context(), m); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, m)
	}
}
