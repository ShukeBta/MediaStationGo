package handler

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type embyPlayingReq struct {
	ItemId        string `json:"ItemId"`
	PositionTicks int64  `json:"PositionTicks"`
	RunTimeTicks  int64  `json:"RunTimeTicks"`
}

func embyPlayingProgressHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		if uid == "" {
			c.Status(http.StatusUnauthorized)
			return
		}
		var req embyPlayingReq
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid playback progress"})
			return
		}
		if req.ItemId == "" {
			req.ItemId = c.Query("ItemId")
		}
		if req.PositionTicks == 0 {
			req.PositionTicks, _ = strconv.ParseInt(c.Query("PositionTicks"), 10, 64)
		}
		if req.RunTimeTicks == 0 {
			req.RunTimeTicks, _ = strconv.ParseInt(c.Query("RunTimeTicks"), 10, 64)
		}
		if req.ItemId == "" {
			c.Status(http.StatusOK)
			return
		}
		clientInfo := embyClientInfoFromRequest(c)
		if svc.Device != nil && svc.Device.IsTerminalKicked(c.Request.Context(), uid, clientInfo.DeviceID, clientInfo.DeviceName, clientInfo.Client) {
			c.Status(http.StatusUnauthorized)
			return
		}
		if err := svc.Emby.RecordProgress(c.Request.Context(), uid, req.ItemId, req.PositionTicks, req.RunTimeTicks); err != nil {
			if errors.Is(err, service.ErrCloudPlaybackNotResolved) {
				if svc.Log != nil {
					svc.Log.Warn("ignored playback progress without successful cloud resolve",
						zap.String("user_id", uid),
						zap.String("media_id", req.ItemId))
				}
			} else {
				if svc.Log != nil {
					svc.Log.Error("record playback progress failed", zap.Error(err))
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": "record playback progress failed"})
				return
			}
		}
		stopped := strings.Contains(strings.ToLower(c.FullPath()+" "+c.Request.URL.Path), "stopped")
		if svc.Sessions != nil {
			svc.Sessions.RecordPlayback(c.Request.Context(), uid, "",
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client,
				c.ClientIP(),
				req.ItemId,
				req.PositionTicks,
				req.RunTimeTicks,
				stopped)
		}
		if svc.Device != nil && !stopped {
			svc.Device.RecordPlayback(c.Request.Context(), uid,
				clientInfo.DeviceID,
				clientInfo.DeviceName,
				clientInfo.Client)
		}
		c.Status(http.StatusNoContent)
	}
}

func embyFavoriteHandler(svc *service.Container, fav bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		mid := c.Param("itemId")
		if uid == "" || mid == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := svc.Emby.SetFavorite(c.Request.Context(), uid, mid, fav); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		out, _ := svc.Emby.Item(c.Request.Context(), mid, uid)
		if out != nil {
			c.JSON(http.StatusOK, out["UserData"])
			return
		}
		c.JSON(http.StatusOK, gin.H{"IsFavorite": fav})
	}
}

func embyMarkPlayedHandler(svc *service.Container, played bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		mid := c.Param("itemId")
		if uid == "" || mid == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if err := svc.Emby.MarkPlayed(c.Request.Context(), uid, mid, played); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if played && svc.Device != nil {
			clientInfo := embyClientInfoFromRequest(c)
			svc.Device.RecordPlayback(c.Request.Context(), uid, clientInfo.DeviceID, clientInfo.DeviceName, clientInfo.Client)
		}
		out, _ := svc.Emby.Item(c.Request.Context(), mid, uid)
		if out != nil {
			c.JSON(http.StatusOK, out["UserData"])
			return
		}
		c.JSON(http.StatusOK, gin.H{"Played": played})
	}
}

func embyHideFromResumeHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid := embyUserID(c)
		mid := c.Param("itemId")
		if mid == "" {
			mid = c.Param("id")
		}
		if uid == "" || mid == "" {
			c.Status(http.StatusBadRequest)
			return
		}
		if len(mid) > 128 {
			embyError(c, http.StatusBadRequest, "Invalid media id")
			return
		}
		// HideFromResume is an action route, so standard clients default to hide.
		// Filmly also sends parameterless calls while synchronizing Resume; those
		// calls are ambiguous and must not mutate the persisted hidden state.
		hide := true
		rawHide := strings.TrimSpace(firstQueryValue(c, "Hide", "hide"))
		isFilmly := embyRequestIsFilmly(c)
		ignoreFilmlyMutation := rawHide == "" && isFilmly
		if rawHide != "" {
			parsed, err := strconv.ParseBool(rawHide)
			if err != nil {
				embyError(c, http.StatusBadRequest, "Invalid Hide value")
				return
			}
			hide = parsed
		}
		if !ignoreFilmlyMutation {
			if err := svc.Emby.SetHiddenFromResume(c.Request.Context(), uid, mid, hide); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		out, err := svc.Emby.Item(c.Request.Context(), mid, uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if out == nil {
			embyError(c, http.StatusNotFound, "item not found")
			return
		}
		if userData, ok := out["UserData"].(map[string]any); ok {
			if isFilmly {
				embyAttachFilmlyUserDataIdentity(userData, mid)
			}
			c.JSON(http.StatusOK, userData)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"PlaybackPositionTicks": 0,
			"PlayCount":             0,
			"IsFavorite":            false,
			"Played":                false,
			"PlayedPercentage":      0,
		})
	}
}
