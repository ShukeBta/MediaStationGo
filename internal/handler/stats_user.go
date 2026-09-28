// Package handler — per-user stats and a play-event recorder.
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// statsUserHandler returns a watch-time summary for one user.
func statsUserHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := c.Param("id")
		var watched int64
		_ = svc.Repo.DB.Model(&model.PlaybackHistory{}).
			Where("user_id = ?", uid).
			Select("COALESCE(SUM(position_ms), 0)").
			Row().Scan(&watched)
		var total int64
		_ = svc.Repo.DB.Model(&model.PlaybackHistory{}).
			Where("user_id = ?", uid).Count(&total).Error
		c.JSON(http.StatusOK, gin.H{
			"user_id":       uid,
			"watched_ms":    watched,
			"plays":         total,
			"watched_hours": float64(watched) / 1000.0 / 3600.0,
		})
	}
}

// statsTopUsersHandler returns the most active users by play count.
func statsTopUsersHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
		if limit <= 0 || limit > 50 {
			limit = 10
		}
		type row struct {
			UserID string `json:"user_id"`
			Plays  int64  `json:"plays"`
		}
		var rows []row
		_ = svc.Repo.DB.Table("playback_histories").
			Select("user_id, COUNT(*) as plays").
			Group("user_id").
			Order("plays desc").
			Limit(limit).Scan(&rows).Error
		// Hydrate usernames in one query.
		ids := make([]string, 0, len(rows))
		for _, r := range rows {
			ids = append(ids, r.UserID)
		}
		nameIdx := map[string]string{}
		if len(ids) > 0 {
			var users []model.User
			_ = svc.Repo.DB.Where("id IN ?", ids).Find(&users).Error
			for _, u := range users {
				nameIdx[u.ID] = u.Username
			}
		}
		out := make([]gin.H, 0, len(rows))
		for _, r := range rows {
			out = append(out, gin.H{
				"user_id":  r.UserID,
				"username": nameIdx[r.UserID],
				"plays":    r.Plays,
			})
		}
		c.JSON(http.StatusOK, gin.H{"items": out})
	}
}

// statsPlayHandler accepts a play event so the Vue analytics panel can
// emit one even when the actual progress write goes through /history.
type playEventReq struct {
	MediaID    string `json:"media_id" binding:"required"`
	PositionMs *int64 `json:"position_ms"`
	DurationMs int64  `json:"duration_ms"`
	SessionID  string `json:"session_id"`
}

func statsPlayHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req playEventReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		media, err := svc.Media.GetMedia(c.Request.Context(), req.MediaID)
		if err != nil || media == nil || !mediaVisibleForRequest(c, svc, media) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		position := int64(0)
		if req.PositionMs != nil {
			position = *req.PositionMs
			if err := svc.Playback.RecordProgressWithVisibility(c.Request.Context(), c.GetString(middleware.CtxUserID),
				req.MediaID, position, req.DurationMs, mediaVisibilityForRequest(c, svc)); err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, service.ErrInvalidPlaybackProgress) {
					status = http.StatusBadRequest
				}
				if errors.Is(err, service.ErrPlaybackMediaUnavailable) {
					status = http.StatusNotFound
				}
				if errors.Is(err, service.ErrCloudPlaybackNotResolved) {
					status = http.StatusConflict
				}
				c.JSON(status, gin.H{"error": err.Error()})
				return
			}
		}
		// A bare analytics event must never replace resume data with zero values.
		recordPlaybackStats(c, svc, req.MediaID, req.SessionID, c.ClientIP(), "Web", position, req.DurationMs, false)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
