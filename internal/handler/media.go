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
	Name      string                     `json:"name" binding:"required"`
	Path      string                     `json:"path"`
	Paths     []string                   `json:"paths"`
	Roots     []service.LibraryRootInput `json:"roots"`
	Type      string                     `json:"type"`
	TitleMode string                     `json:"title_mode"`
}

type updateLibraryReq struct {
	Enabled         *bool   `json:"enabled"`
	TitleMode       *string `json:"title_mode"`
	GenerateArtwork *bool   `json:"generate_artwork"`
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
		l, err := svc.Media.CreateLibraryWithRootsAndTitleMode(c.Request.Context(), req.Name, req.Type, req.TitleMode, roots)
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "title_mode") {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get("ctx_user_id")
		svc.Audit.Record(c.Request.Context(), toString(uid), "library.create", l.ID, c.ClientIP(), l.Path)
		// Refresh fsnotify watcher to pick up the new library root.
		go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		c.JSON(http.StatusCreated, l)
	}
}

func updateLibraryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req updateLibraryReq
		if err := c.ShouldBindJSON(&req); err != nil || (req.Enabled == nil && req.TitleMode == nil && req.GenerateArtwork == nil) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "enabled, title_mode or generate_artwork is required"})
			return
		}
		lib, err := svc.Media.UpdateLibrary(c.Request.Context(), c.Param("id"), service.LibraryUpdateInput{
			Enabled:         req.Enabled,
			TitleMode:       req.TitleMode,
			GenerateArtwork: req.GenerateArtwork,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if strings.Contains(err.Error(), "title_mode") {
				status = http.StatusBadRequest
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		if lib == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		uid, _ := c.Get("ctx_user_id")
		if svc.Audit != nil {
			detail := "enabled=" + strconv.FormatBool(lib.Enabled) + ",title_mode=" + lib.TitleMode + ",generate_artwork=" + strconv.FormatBool(lib.GenerateArtwork)
			svc.Audit.Record(c.Request.Context(), toString(uid), "library.update", lib.ID, c.ClientIP(), detail)
		}
		if req.GenerateArtwork != nil && *req.GenerateArtwork && svc.GeneratedArtwork != nil {
			if _, queueErr := svc.GeneratedArtwork.QueueMissingForLibrary(c.Request.Context(), lib.ID); queueErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": queueErr.Error()})
				return
			}
		}
		if req.GenerateArtwork != nil && !*req.GenerateArtwork && svc.GeneratedArtwork != nil {
			if _, cancelErr := svc.GeneratedArtwork.CancelLibrary(c.Request.Context(), lib.ID); cancelErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": cancelErr.Error()})
				return
			}
		}
		if svc.Watcher != nil {
			go func() { _ = svc.Watcher.Refresh(context.Background()) }()
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
			items = mediaSliceForResponse(c, items)
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
		items = mediaItemsForResponse(c, items)
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
		c.JSON(http.StatusOK, mediaForResponse(c, *m))
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
		q := strings.TrimSpace(c.Query("q"))
		if len([]rune(q)) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "搜索词长度不能超过 100 个字符"})
			return
		}
		groupVersions := c.DefaultQuery("group_versions", "1") != "0"
		if c.Query("page") != "" || c.Query("page_size") != "" {
			page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
			size, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
			if c.Query("group_series") == "1" {
				items, total, err := svc.Media.SearchMediaVisibleSeriesPage(c.Request.Context(), q, page, size, mediaVisibilityForRequest(c, svc))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"items":     seriesCardsForResponse(c, items),
					"total":     total,
					"page":      page,
					"page_size": size,
				})
				return
			}
			if !groupVersions {
				items, total, err := svc.Media.SearchMediaVisiblePage(c.Request.Context(), q, page, size, mediaVisibilityForRequest(c, svc))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{
					"items":     mediaSliceForResponse(c, items),
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
				"items":     mediaItemsForResponse(c, items),
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
			c.JSON(http.StatusOK, gin.H{"items": mediaSliceForResponse(c, items)})
			return
		}
		items, err := svc.Media.SearchMediaVisibleGrouped(c.Request.Context(), q, limit, mediaVisibilityForRequest(c, svc))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": mediaItemsForResponse(c, items)})
	}
}

type mediaWorkSearchRequest struct {
	Queries   []string `json:"queries"`
	LibraryID string   `json:"library_id"`
	Limit     int      `json:"limit"`
}

func searchMediaWorksHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req mediaWorkSearchRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if strings.TrimSpace(req.LibraryID) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "library_id is required"})
			return
		}
		if len(req.Queries) == 0 || len(req.Queries) > 5 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "queries must contain between 1 and 5 entries"})
			return
		}
		for _, query := range req.Queries {
			if strings.TrimSpace(query) == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "queries must not contain empty entries"})
				return
			}
			if len([]rune(query)) > 100 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "搜索词长度不能超过 100 个字符"})
				return
			}
		}
		if req.Limit <= 0 {
			req.Limit = 100
		} else if req.Limit > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "limit cannot exceed 100"})
			return
		}
		items, total, err := svc.Media.SearchMediaWorkCandidatesVisible(
			c.Request.Context(), req.Queries, req.LibraryID, req.Limit, mediaVisibilityForRequest(c, svc),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items": seriesCardsForResponse(c, items),
			"total": total,
		})
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
		err = svc.Stream.ServeFileForUser(c.Writer, c.Request, c.Param("id"), currentUserID(c))
		if errors.Is(err, service.ErrMediaNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if errors.Is(err, service.ErrCloudPlaybackDisabled) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, service.ErrCloudPlaybackUnavailable) ||
			errors.Is(err, service.ErrCloudPlaybackResolveFailed) {
			c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
			return
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
}
