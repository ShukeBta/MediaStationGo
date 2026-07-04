package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func embyViewsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("userId")
		if uid == "" {
			uid = embyUserID(c)
		}
		out, err := svc.Emby.Views(c.Request.Context(), uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		embyAttachRequestTokenToMediaSources(c, out)
		c.JSON(http.StatusOK, out)
	}
}

func embyVirtualFoldersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		libs, err := svc.Repo.Library.List(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		libs = service.FilterDisplayCloudLibraries(c.Request.Context(), svc.Repo, libs)
		uid := embyUserID(c)
		visibility := service.UserDefaultMediaVisibility(c.Request.Context(), svc.Repo, uid)
		out := make([]gin.H, 0, len(libs))
		for _, lib := range libs {
			if !service.LibraryVisibleForUser(c.Request.Context(), svc.Repo, lib, visibility) {
				continue
			}
			collectionType := "movies"
			switch lib.Type {
			case "tv", "anime", "variety":
				collectionType = "tvshows"
			case "music":
				collectionType = "music"
			}
			out = append(out, gin.H{
				"Name":               lib.Name,
				"Locations":          []string{lib.Path},
				"CollectionType":     collectionType,
				"ItemId":             lib.ID,
				"Id":                 lib.ID,
				"PrimaryImageItemId": lib.ID,
				"RefreshStatus":      "Idle",
				"LibraryOptions":     gin.H{},
			})
		}
		c.JSON(http.StatusOK, out)
	}
}
