package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

type settingReq struct {
	Key   string `json:"key" binding:"required"`
	Value string `json:"value"`
}

func listSettingsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		settings, err := svc.Repo.Setting.All(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, settings)
	}
}

func updateSettingHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req settingReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		oldValue := ""
		if req.Key == service.AdultLibraryIDsSettingKey {
			oldValue, _ = svc.Repo.Setting.Get(c.Request.Context(), req.Key)
		}
		if err := svc.Repo.Setting.Set(c.Request.Context(), req.Key, req.Value); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		oldAdultLibraryIDs := service.DecodeAllowedLibraryIDs(oldValue)
		newAdultLibraryIDs := service.DecodeAllowedLibraryIDs(req.Value)
		if req.Key == service.AdultLibraryIDsSettingKey && len(oldAdultLibraryIDs) == 0 && len(newAdultLibraryIDs) > 0 {
			_ = svc.Repo.DB.WithContext(c.Request.Context()).Model(&model.User{}).Where("hide_adult = ?", false).Update("hide_adult", true).Error
		}
		service.ApplyRuntimeSetting(svc.Cfg, req.Key, req.Value)
		if svc.FFprobe != nil && (req.Key == "ffprobe.max_concurrent" || req.Key == "app.ffprobe_max_concurrent") {
			svc.FFprobe.SetMaxConcurrent(svc.Cfg.App.FFprobeMaxConcurrent)
		}
		if req.Key == "transcode.enabled" && !svc.Cfg.Transcoder.Enabled {
			svc.Transcoder.StopAll()
		}
		if req.Key == "transcode.hw_enabled" || req.Key == "transcode.hw_accel" || req.Key == "transcoder.hardware_accel" || req.Key == "transcoder.encoder" {
			svc.Transcoder.StopAll()
		}
		if req.Key == "cloud.auto_sync_enabled" && !service.ParseBoolSetting(req.Value, false) && svc.Scan != nil {
			_ = svc.Scan.CancelAllCloudScans()
		}
		c.Status(http.StatusNoContent)
	}
}

func recentLogsHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := svc.Repo.Log.Recent(c.Request.Context(), 200)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, rows)
	}
}
