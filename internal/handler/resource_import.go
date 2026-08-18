package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
	"github.com/gin-gonic/gin"
)

func registerAuthedResourceImportRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.POST("/libraries/:id/resource-searches", resourceSearchHandler(svc))
	authed.POST("/libraries/:id/manual-resource-previews", manualResourcePreviewHandler(svc))
	authed.POST("/libraries/:id/resource-imports", createResourceImportHandler(svc))
	authed.POST("/media/:id/episode-replenishments", createEpisodeReplenishmentHandler(svc))
	authed.GET("/libraries/:id/resource-imports", listLibraryResourceImportsHandler(svc))
	authed.GET("/resource-imports", listResourceImportsHandler(svc))
	authed.GET("/resource-imports/:id", getResourceImportHandler(svc))
	authed.DELETE("/resource-imports/:id", deleteFailedResourceImportHandler(svc))
	authed.POST("/resource-imports/:id/cancel", cancelResourceImportHandler(svc))
	authed.POST("/resource-imports/:id/retry", retryResourceImportHandler(svc))
}

func manualResourcePreviewHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		library, ok := visibleResourceImportLibrary(c, svc, c.Param("id"))
		if !ok {
			return
		}
		var in struct {
			Input  string `json:"input"`
			Title  string `json:"title"`
			RootID string `json:"root_id"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		root, err := selectResourceImportRoot(*library, in.RootID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := resourceImport.PrepareManual(c.Request.Context(), middleware.GetUserID(c), *library, root, in.Input, in.Title)
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func resourceSearchHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		library, ok := visibleResourceImportLibrary(c, svc, c.Param("id"))
		if !ok {
			return
		}
		var in service.ResourceSearchInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		root, err := selectResourceImportRoot(*library, in.RootID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		result, err := resourceImport.Search(c.Request.Context(), middleware.GetUserID(c), *library, root, in)
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func createResourceImportHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		library, ok := visibleResourceImportLibrary(c, svc, c.Param("id"))
		if !ok {
			return
		}
		var in service.ResourceImportCreateInput
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		in.IsAdmin = middleware.IsAdmin(c)
		root, err := selectResourceImportRoot(*library, in.RootID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		task, err := resourceImport.Create(c.Request.Context(), middleware.GetUserID(c), *library, root, in)
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func createEpisodeReplenishmentHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		var in struct {
			Input string `json:"input"`
		}
		if err := c.ShouldBindJSON(&in); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}
		task, err := resourceImport.ReplenishEpisodes(
			c.Request.Context(), middleware.GetUserID(c), c.Param("id"), in.Input,
		)
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func listLibraryResourceImportsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		library, ok := visibleResourceImportLibrary(c, svc, c.Param("id"))
		if !ok {
			return
		}
		result, err := resourceImport.List(c.Request.Context(), middleware.GetUserID(c), middleware.IsAdmin(c), service.ResourceImportListFilter{
			LibraryID: library.ID,
			Status:    c.Query("status"),
			Page:      queryInt(c, "page"),
			PageSize:  queryInt(c, "page_size"),
		})
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func listResourceImportsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		isAdmin := middleware.IsAdmin(c)
		result, err := resourceImport.List(c.Request.Context(), middleware.GetUserID(c), isAdmin, service.ResourceImportListFilter{
			LibraryID: c.Query("library_id"),
			UserID:    c.Query("user_id"),
			Status:    c.Query("status"),
			Page:      queryInt(c, "page"),
			PageSize:  queryInt(c, "page_size"),
		})
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	}
}

func getResourceImportHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		task, err := resourceImport.Get(c.Request.Context(), middleware.GetUserID(c), middleware.IsAdmin(c), c.Param("id"))
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusOK, task)
	}
}

func cancelResourceImportHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		task, err := resourceImport.Cancel(c.Request.Context(), middleware.GetUserID(c), middleware.IsAdmin(c), c.Param("id"))
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusOK, task)
	}
}

func retryResourceImportHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		task, err := resourceImport.Retry(c.Request.Context(), middleware.GetUserID(c), middleware.IsAdmin(c), c.Param("id"))
		if err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func deleteFailedResourceImportHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		resourceImport, ok := requireResourceImportService(c, svc)
		if !ok {
			return
		}
		if err := resourceImport.DeleteFailed(c.Request.Context(), middleware.GetUserID(c), middleware.IsAdmin(c), c.Param("id")); err != nil {
			writeResourceImportError(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func requireResourceImportService(c *gin.Context, svc *service.Container) (*service.ResourceImportService, bool) {
	if svc == nil || svc.ResourceImport == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "资源搜索入库功能未启用"})
		return nil, false
	}
	return svc.ResourceImport, true
}

func visibleResourceImportLibrary(c *gin.Context, svc *service.Container, id string) (*model.Library, bool) {
	if svc == nil || svc.Repo == nil || svc.Repo.Library == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "library service unavailable"})
		return nil, false
	}
	library, err := svc.Repo.Library.FindByID(c.Request.Context(), strings.TrimSpace(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil, false
	}
	if library == nil || !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, *library, mediaVisibilityForRequest(c, svc)) {
		c.JSON(http.StatusNotFound, gin.H{"error": "library not found"})
		return nil, false
	}
	return library, true
}

func selectResourceImportRoot(library model.Library, rootID string) (model.LibraryRoot, error) {
	enabled := make([]model.LibraryRoot, 0, len(library.Roots))
	for _, root := range library.Roots {
		if root.Enabled {
			enabled = append(enabled, root)
		}
	}
	rootID = strings.TrimSpace(rootID)
	if rootID != "" {
		for _, root := range enabled {
			if root.ID == rootID {
				return root, nil
			}
		}
		return model.LibraryRoot{}, errors.New("media library root not found")
	}
	if len(enabled) == 1 {
		return enabled[0], nil
	}
	if len(enabled) == 0 {
		return model.LibraryRoot{}, errors.New("media library has no enabled root")
	}
	return model.LibraryRoot{}, errors.New("root_id is required for a multi-root library")
}

func writeResourceImportError(c *gin.Context, err error) {
	if errors.Is(err, service.ErrMediaVersionForbidden) {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}
	if errors.Is(err, service.ErrResourceImportDeleteNotAllowed) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}
	var search *service.ResourceSearchError
	if errors.As(err, &search) {
		status := search.HTTPStatus()
		if status < 400 || status > 599 {
			status = http.StatusBadGateway
		}
		c.JSON(status, gin.H{
			"error": gin.H{
				"code":         search.Code,
				"message":      search.Error(),
				"capabilities": search.Capabilities,
			},
		})
		return
	}
	var duplicate *service.ResourceImportDuplicateError
	if errors.As(err, &duplicate) {
		c.JSON(http.StatusConflict, gin.H{
			"error": gin.H{
				"code": "duplicate_media", "message": duplicate.Message,
				"can_force": duplicate.Duplicate.CanForce,
				"media_id":  duplicate.Duplicate.MediaID,
				"title":     duplicate.Duplicate.Title,
				"reason":    duplicate.Duplicate.Reason,
			},
			"message":   duplicate.Message,
			"can_force": duplicate.Duplicate.CanForce,
			"media_id":  duplicate.Duplicate.MediaID,
		})
		return
	}
	var pipelineStatus interface{ HTTPStatus() int }
	if errors.As(err, &pipelineStatus) {
		if code := pipelineStatus.HTTPStatus(); code == http.StatusBadRequest || code == http.StatusConflict {
			c.JSON(code, gin.H{"error": gin.H{"message": err.Error()}})
			return
		}
	}
	status := http.StatusBadGateway
	message := err.Error()
	if strings.Contains(message, "not found") {
		status = http.StatusNotFound
	} else if strings.Contains(message, "expired") || strings.Contains(message, "required") || strings.Contains(message, "out of range") || strings.Contains(message, "unsupported") || strings.Contains(message, "仅支持") || strings.Contains(message, "无效") {
		status = http.StatusBadRequest
	} else if strings.Contains(message, "already final") || strings.Contains(message, "not retryable") || strings.Contains(message, "无法取消") || strings.Contains(message, "尚未配置") {
		status = http.StatusConflict
	}
	c.JSON(status, gin.H{"error": gin.H{"message": message}})
}

func queryInt(c *gin.Context, key string) int {
	value, _ := strconv.Atoi(strings.TrimSpace(c.Query(key)))
	return value
}
