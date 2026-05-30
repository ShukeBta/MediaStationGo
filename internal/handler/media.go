// Package handler — library / media HTTP endpoints.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type createLibraryReq struct {
	Name string `json:"name" binding:"required"`
	Path string `json:"path" binding:"required"`
	Type string `json:"type"`
}

func listLibrariesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libs, err := svc.Media.ListLibraries(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		role, _ := c.Get(middleware.CtxUserRole)
		includeHidden := role == "admin" && (c.Query("include_hidden") == "1" || c.Query("all") == "1")
		if !includeHidden {
			visibility := mediaVisibilityForRequest(c, svc)
			filtered := libs[:0]
			for _, lib := range libs {
				if service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, lib, visibility) {
					filtered = append(filtered, lib)
				}
			}
			libs = filtered
		}
		c.JSON(http.StatusOK, libs)
	}
}

func createLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createLibraryReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		l, err := svc.Media.CreateLibrary(c.Request.Context(), req.Name, req.Path, req.Type)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get("ctx_user_id")
		svc.Audit.Record(c.Request.Context(), toString(uid), "library.create", l.ID, c.ClientIP(), l.Path)
		// Refresh fsnotify watcher to pick up the new library root.
		go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		c.JSON(http.StatusCreated, l)
	}
}

func deleteLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if err := svc.Media.DeleteLibrary(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get("ctx_user_id")
		svc.Audit.Record(c.Request.Context(), toString(uid), "library.delete", id, c.ClientIP(), "")
		go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		c.Status(http.StatusNoContent)
	}
}

func scanLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		// Run synchronously: small libraries return immediately, big ones can
		// hit the (configurable) HTTP timeout. A future task queue can move
		// this to a background worker.
		res, err := svc.Scan.ScanLibrary(c.Request.Context(), id)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, res)
	}
}

func listMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
		items, total, err := svc.Media.ListMediaVisible(c.Request.Context(), id, page, size, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items":     items,
			"total":     total,
			"page":      page,
			"page_size": size,
		})
	}
}

func getMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		m, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if m == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if !mediaVisibleForRequest(c, svc, m) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, m)
	}
}

func searchMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Query("q")
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		items, err := svc.Media.SearchMediaVisible(c.Request.Context(), q, limit, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func streamHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		m, err := svc.Media.GetMedia(c.Request.Context(), c.Param("id"))
		if err != nil || m == nil || !mediaVisibleForRequest(c, svc, m) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		err = svc.Stream.ServeFile(c.Writer, c.Request, c.Param("id"))
		if errors.Is(err, service.ErrMediaNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
}
