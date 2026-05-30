// Package handler — playback history / favourites / playlists endpoints.
package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// ─── History ────────────────────────────────────────────────────────────────

type progressReq struct {
	MediaID    string `json:"media_id" binding:"required"`
	PositionMs int64  `json:"position_ms"`
	DurationMs int64  `json:"duration_ms"`
}

func recordProgressHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req progressReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		if err := svc.Playback.RecordProgress(
			c.Request.Context(), uid.(string), req.MediaID, req.PositionMs, req.DurationMs,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func recentHistoryHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		items, err := svc.Playback.RecentHistory(c.Request.Context(), uid.(string), 30)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		visibility := mediaVisibilityForRequest(c, svc)
		filtered := make([]service.HistoryItem, 0, len(items))
		for _, item := range items {
			if item.Media == nil || visibility.Allows(item.Media) {
				filtered = append(filtered, item)
			}
		}
		c.JSON(http.StatusOK, gin.H{"items": filtered})
	}
}

// ─── Favourites ─────────────────────────────────────────────────────────────

func toggleFavouriteHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		state, err := svc.Playback.ToggleFavourite(
			c.Request.Context(), uid.(string), c.Param("id"),
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"favourite": state})
	}
}

func listFavouritesHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		items, err := svc.Playback.ListFavourites(c.Request.Context(), uid.(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		visibility := mediaVisibilityForRequest(c, svc)
		filtered := make([]any, 0, len(items))
		for i := range items {
			if visibility.Allows(&items[i]) {
				filtered = append(filtered, items[i])
			}
		}
		c.JSON(http.StatusOK, gin.H{"items": filtered})
	}
}

// ─── Playlists ──────────────────────────────────────────────────────────────

type createPlaylistReq struct {
	Name     string `json:"name" binding:"required"`
	IsPublic bool   `json:"is_public"`
}

func createPlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req createPlaylistReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		pl, err := svc.Playback.CreatePlaylist(
			c.Request.Context(), uid.(string), req.Name, req.IsPublic,
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, pl)
	}
}

func listPlaylistsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, _ := c.Get(middleware.CtxUserID)
		items, err := svc.Playback.ListPlaylists(c.Request.Context(), uid.(string))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": items})
	}
}

func getPlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		detail, err := svc.Playback.GetPlaylist(c.Request.Context(), c.Param("id"))
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		uid, _ := c.Get(middleware.CtxUserID)
		role, _ := c.Get(middleware.CtxUserRole)
		if !detail.Playlist.IsPublic && detail.Playlist.UserID != uid.(string) && role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		visibility := mediaVisibilityForRequest(c, svc)
		filtered := detail.Items[:0]
		for i := range detail.Items {
			if visibility.Allows(&detail.Items[i]) {
				filtered = append(filtered, detail.Items[i])
			}
		}
		detail.Items = filtered
		c.JSON(http.StatusOK, detail)
	}
}

type playlistItemReq struct {
	MediaID string `json:"media_id" binding:"required"`
}

func addPlaylistItemHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req playlistItemReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := svc.Playback.AddToPlaylist(
			c.Request.Context(), c.Param("id"), req.MediaID,
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func removePlaylistItemHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Playback.RemoveFromPlaylist(
			c.Request.Context(), c.Param("id"), c.Param("media_id"),
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func deletePlaylistHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := svc.Playback.DeletePlaylist(
			c.Request.Context(), c.Param("id"),
		); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
