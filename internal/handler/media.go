// Package handler — library / media HTTP endpoints.
package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type createLibraryReq struct {
	Name     string                     `json:"name" binding:"required"`
	Path     string                     `json:"path"`
	Paths    []string                   `json:"paths"`
	Roots    []service.LibraryRootInput `json:"roots"`
	Type     string                     `json:"type"`
	CoverURL string                     `json:"cover_url"`
}

func listLibrariesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		libs, err := svc.Media.ListLibraries(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		libs = service.FilterDeprecatedNativeCloudLibraries(libs)
		role, _ := c.Get(middleware.CtxUserRole)
		includeHidden := role == "admin" && (c.Query("include_hidden") == "1" || c.Query("all") == "1")
		if !includeHidden {
			libs = service.FilterDisplayCloudLibraries(c.Request.Context(), svc.Repo, libs)
			visibility := mediaVisibilityForRequest(c, svc)
			filtered := libs[:0]
			for _, lib := range libs {
				if service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, lib, visibility) {
					filtered = append(filtered, lib)
				}
			}
			libs = filtered
		} else {
			libs = service.FilterMergedCloudAutoCategoryLibraries(libs)
			libs = service.NormalizeCloudLibraryDisplayNames(libs)
		}
		c.JSON(http.StatusOK, libs)
	}
}

func getLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		lib, err := svc.Repo.Library.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if lib == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		libs := service.FilterDeprecatedNativeCloudLibraries([]model.Library{*lib})
		if len(libs) == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		role, _ := c.Get(middleware.CtxUserRole)
		includeHidden := role == "admin" && (c.Query("include_hidden") == "1" || c.Query("all") == "1")
		if includeHidden {
			c.JSON(http.StatusOK, service.NormalizeCloudLibraryDisplayNames(libs)[0])
			return
		}
		libs = service.FilterDisplayCloudLibraries(c.Request.Context(), svc.Repo, libs)
		if len(libs) == 0 || !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, libs[0], mediaVisibilityForRequest(c, svc)) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusOK, libs[0])
	}
}

func createLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createLibraryReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		roots := req.Roots
		if len(roots) == 0 {
			for _, path := range req.Paths {
				roots = append(roots, service.LibraryRootInput{Path: path})
			}
		}
		if len(roots) == 0 && strings.TrimSpace(req.Path) != "" {
			roots = append(roots, service.LibraryRootInput{Path: req.Path})
		}
		l, err := svc.Media.CreateLibraryWithRootsAndCover(c.Request.Context(), req.Name, req.Type, req.CoverURL, roots)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get("ctx_user_id")
		svc.Audit.Record(c.Request.Context(), toString(uid), "library.create", l.ID, c.ClientIP(), l.Path)
		// Refresh fsnotify watcher to pick up the new library root, then perform
		// an initial scan. Without the scan a newly-created library remained
		// empty until the operator pressed the separate "扫描" action.
		if svc.Watcher != nil {
			go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		}
		if len(l.Roots) == 0 {
			queueLibraryRootScan(svc, l.ID, "")
		} else {
			for _, root := range l.Roots {
				if root.Enabled {
					queueLibraryRootScan(svc, l.ID, root.ID)
				}
			}
		}
		c.JSON(http.StatusCreated, l)
	}
}

type updateLibraryReq struct {
	CoverURL string `json:"cover_url"`
}

func updateLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req updateLibraryReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := svc.Media.UpdateLibraryCover(c.Request.Context(), c.Param("id"), req.CoverURL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		lib, err := svc.Repo.Library.FindByID(c.Request.Context(), c.Param("id"))
		if err != nil || lib == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "library not found"})
			return
		}
		c.JSON(http.StatusOK, lib)
	}
}

func deleteLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		if lib, err := svc.Repo.Library.FindByID(c.Request.Context(), id); err == nil && lib != nil {
			if _, ok := service.ParseCloudLibraryMount(lib.Path); ok && svc.Scan != nil {
				_ = svc.Scan.CancelCloudScan(id)
			}
		}
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

func listMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.Param("id")
		page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
		size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
		groupVersions := c.DefaultQuery("group_versions", "1") != "0"
		if !groupVersions {
			items, total, err := svc.Media.ListMediaVisible(c.Request.Context(), id, page, size, mediaVisibilityForRequest(c, svc))
			if err != nil {
				writeInternalOrCanceled(c, err)
				return
			}
			if items == nil {
				items = []model.Media{}
			}
			c.JSON(http.StatusOK, gin.H{
				"items":     items,
				"total":     total,
				"page":      page,
				"page_size": size,
			})
			return
		}
		items, total, err := svc.Media.ListMediaVisibleGrouped(c.Request.Context(), id, page, size, mediaVisibilityForRequest(c, svc))
		if err != nil {
			writeInternalOrCanceled(c, err)
			return
		}
		if items == nil {
			items = []service.MediaItem{}
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

func updateMediaMetadataHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req service.MediaMetadataUpdate
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		m, err := svc.Media.UpdateMetadata(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				status = http.StatusNotFound
			} else if strings.Contains(strings.ToLower(err.Error()), "required") {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, m)
	}
}

func searchMediaHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		q := c.Query("q")
		groupVersions := c.DefaultQuery("group_versions", "1") != "0"
		if c.Query("page") != "" || c.Query("page_size") != "" {
			page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
			size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
			if !groupVersions {
				items, total, err := svc.Media.SearchMediaVisiblePage(c.Request.Context(), q, page, size, mediaVisibilityForRequest(c, svc))
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
				return
			}
			items, total, err := svc.Media.SearchMediaVisiblePageGrouped(c.Request.Context(), q, page, size, mediaVisibilityForRequest(c, svc))
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
			return
		}
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
		if !groupVersions {
			items, err := svc.Media.SearchMediaVisible(c.Request.Context(), q, limit, mediaVisibilityForRequest(c, svc))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"items": items})
			return
		}
		items, err := svc.Media.SearchMediaVisibleGrouped(c.Request.Context(), q, limit, mediaVisibilityForRequest(c, svc))
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
		if !enforceScopedPlaybackToken(c, m.ID) {
			return
		}
		err = svc.Stream.ServeFile(c.Writer, c.Request, c.Param("id"))
		if errors.Is(err, service.ErrMediaNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if errors.Is(err, service.ErrCloudPlaybackDisabled) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrCloudPlaybackUnavailable) {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
}
