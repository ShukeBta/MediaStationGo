package handler

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const maxPlaybackTrackKeyLength = 512

type embyPlaybackPreferenceInput struct {
	SubtitleEnabled  *bool   `json:"subtitle_enabled"`
	SubtitleTrackKey *string `json:"subtitle_track_key"`
	AudioTrackKey    *string `json:"audio_track_key"`
}

type embyPlaybackPreferenceResponse struct {
	Configured       bool    `json:"configured"`
	SubtitleEnabled  bool    `json:"subtitle_enabled"`
	SubtitleTrackKey *string `json:"subtitle_track_key"`
	AudioTrackKey    *string `json:"audio_track_key"`
}

func embyPlaybackPreferenceHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		mediaID, ok := validPlaybackPreferenceMediaID(c)
		if !ok {
			return
		}
		row, err := svc.Repo.MediaPlaybackPreference.FindByUserAndMedia(
			c.Request.Context(),
			embyUserID(c),
			mediaID,
		)
		if err != nil {
			embyError(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.JSON(http.StatusOK, playbackPreferenceResponse(row))
	}
}

func embyUpdatePlaybackPreferenceHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		mediaID, ok := validPlaybackPreferenceMediaID(c)
		if !ok {
			return
		}
		var input embyPlaybackPreferenceInput
		if err := c.ShouldBindJSON(&input); err != nil {
			embyError(c, http.StatusBadRequest, err.Error())
			return
		}
		if input.SubtitleEnabled == nil && input.SubtitleTrackKey == nil && input.AudioTrackKey == nil {
			embyError(c, http.StatusBadRequest, "At least one playback preference field is required")
			return
		}

		userID := embyUserID(c)
		row, err := svc.Repo.MediaPlaybackPreference.FindByUserAndMedia(
			c.Request.Context(),
			userID,
			mediaID,
		)
		if err != nil {
			embyError(c, http.StatusInternalServerError, err.Error())
			return
		}
		if row == nil {
			row = &model.UserMediaPlaybackPreference{
				UserID:          userID,
				MediaID:         mediaID,
				SubtitleEnabled: true,
			}
		}
		if input.SubtitleEnabled != nil {
			row.SubtitleEnabled = *input.SubtitleEnabled
		}
		if input.SubtitleTrackKey != nil {
			row.SubtitleTrackKey, err = normalizePlaybackTrackKey(*input.SubtitleTrackKey)
			if err != nil {
				embyError(c, http.StatusBadRequest, err.Error())
				return
			}
		}
		if input.AudioTrackKey != nil {
			row.AudioTrackKey, err = normalizePlaybackTrackKey(*input.AudioTrackKey)
			if err != nil {
				embyError(c, http.StatusBadRequest, err.Error())
				return
			}
		}
		if err := svc.Repo.MediaPlaybackPreference.Upsert(c.Request.Context(), row); err != nil {
			embyError(c, http.StatusInternalServerError, err.Error())
			return
		}
		c.JSON(http.StatusOK, playbackPreferenceResponse(row))
	}
}

func validPlaybackPreferenceMediaID(c *gin.Context) (string, bool) {
	mediaID := strings.TrimSpace(c.Param("id"))
	if mediaID == "" || len(mediaID) > 128 {
		embyError(c, http.StatusBadRequest, "Invalid media id")
		return "", false
	}
	return mediaID, true
}

func normalizePlaybackTrackKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", fmt.Errorf("playback track key must not be empty")
	}
	if len(key) > maxPlaybackTrackKeyLength {
		return "", fmt.Errorf("playback track key is too long")
	}
	return key, nil
}

func playbackPreferenceResponse(row *model.UserMediaPlaybackPreference) embyPlaybackPreferenceResponse {
	if row == nil {
		return embyPlaybackPreferenceResponse{
			Configured:      false,
			SubtitleEnabled: true,
		}
	}
	return embyPlaybackPreferenceResponse{
		Configured:       true,
		SubtitleEnabled:  row.SubtitleEnabled,
		SubtitleTrackKey: optionalPlaybackTrackKey(row.SubtitleTrackKey),
		AudioTrackKey:    optionalPlaybackTrackKey(row.AudioTrackKey),
	}
}

func optionalPlaybackTrackKey(raw string) *string {
	key := strings.TrimSpace(raw)
	if key == "" {
		return nil
	}
	return &key
}
