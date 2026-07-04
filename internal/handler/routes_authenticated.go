// Package handler — authenticated application routes.
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerAuthenticatedRoutes(api *gin.RouterGroup, cfg *config.Config, svc *service.Container) {
	authed := api.Group("/")
	authed.Use(middleware.AuthRequired(cfg.Secrets.JWTSecret))
	authed.Use(activeUserRequired(svc))

	registerAuthedUserAndLicenseRoutes(authed, svc)
	registerAuthedLibraryRoutes(authed, svc)
	registerAuthedMediaRoutes(authed, svc)
	registerAuthedPlaybackAndProxyRoutes(authed, svc)
	registerAuthedCollectionRoutes(authed, svc)
	registerAuthedDownloadRoutes(authed, svc)
	registerAuthedSubscriptionRoutes(authed, svc)
	registerAuthedStatsDiscoveryAndAIRoutes(authed, svc)
	registerAuthedFileRoutes(authed, svc)
	registerAuthedDLNARoutes(authed, svc)
	registerAuthedSTRMRoutes(authed, svc)
	registerAuthedDuplicateRoutes(authed, svc)
	registerAuthedSiteRoutes(authed, svc)
	registerAuthedRecycleAndRealtimeRoutes(authed, svc)
	registerAuthedSchedulerRoutes(authed, svc)
	registerAuthedUISurfaceRoutes(authed, svc)
	registerAuthedSearchRoutes(authed, svc)
	registerAuthedSystemExtraRoutes(authed, svc)
	registerAuthedStatsExtraRoutes(authed, svc)
	registerAuthedSitesExtraRoutes(authed, svc)
	registerAuthedSubscriptionExtraRoutes(authed, svc)
	registerAuthedPlaylistExtraRoutes(authed, svc)
	registerAuthedDLNAControlRoutes(authed, svc)
	registerAuthedFavoriteAndMediaActionRoutes(authed, svc)
	registerAuthedPlaybackExtraRoutes(authed, svc)
	registerAuthedDownloadOpsRoutes(authed, svc)
	registerAuthedAssistantRoutes(authed, svc)
}
