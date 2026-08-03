package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

const hideCommunityLinksSettingKey = "ui.hide_community_links_for_users"

func publicUIConfigHandler(svc *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		hide := false
		if svc != nil && svc.Repo != nil && svc.Repo.Setting != nil {
			if value, err := svc.Repo.Setting.Get(c.Request.Context(), hideCommunityLinksSettingKey); err == nil {
				hide = value == "true" || value == "1"
			}
		}
		c.JSON(http.StatusOK, gin.H{"hide_community_links_for_users": hide})
	}
}
