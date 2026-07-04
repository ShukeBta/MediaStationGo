package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func listLibraryRootsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		roots, err := svc.Media.ListLibraryRoots(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, roots)
	}
}

func createLibraryRootHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req service.LibraryRootInput
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		root, err := svc.Media.AddLibraryRoot(c.Request.Context(), c.Param("id"), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		c.JSON(http.StatusCreated, root)
	}
}

func updateLibraryRootHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req service.LibraryRootInput
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		root, err := svc.Media.UpdateLibraryRoot(c.Request.Context(), c.Param("id"), c.Param("root_id"), req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if root == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "library root not found"})
			return
		}
		go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		c.JSON(http.StatusOK, root)
	}
}

func deleteLibraryRootHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Media.DeleteLibraryRoot(c.Request.Context(), c.Param("id"), c.Param("root_id")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		go func() { _ = svc.Watcher.Refresh(context.Background()) }()
		c.Status(http.StatusNoContent)
	}
}
