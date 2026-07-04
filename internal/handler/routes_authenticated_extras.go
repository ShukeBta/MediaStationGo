package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerAuthedUISurfaceRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/media/recent", recentMediaHandler(svc))
	authed.GET("/media/stats", mediaStatsHandler(svc))

	authed.GET("/watch-history", historyListHandler(svc))
	authed.GET("/watch-history/stats", historyStatsHandler(svc))
	authed.GET("/watch-history/continue", historyContinueHandler(svc))
	authed.DELETE("/watch-history", historyDeleteHandler(svc))
	authed.DELETE("/watch-history/:id", historyDeleteOneHandler(svc))

	authed.GET("/discover/sections", requirePermission(svc, "can_view_discover"), discoverSectionsHandler(svc))
	authed.GET("/discover/feed", requirePermission(svc, "can_view_discover"), discoverFeedHandler(svc))
	authed.GET("/discover/search", requirePermission(svc, "can_view_discover"), discoverSearchHandler(svc))

	authed.GET("/system/info", systemInfoHandler(svc))
	authed.GET("/system/status", systemStatusHandler(svc))
	authed.GET("/system/scheduler", systemSchedulerHandler(svc))

	authed.GET("/stats/overview", statsOverviewHandler(svc))
	authed.GET("/stats/trend", statsTrendHandler(svc))
	authed.GET("/stats/top-content", statsTopContentHandler(svc))
	authed.GET("/stats/libraries", statsLibrariesHandler(svc))
	authed.GET("/stats/monitor", statsMonitorHandler(svc))

	authed.GET("/play-profiles", listPlayProfilesHandler(svc))
	authed.POST("/play-profiles", createPlayProfileHandler(svc))
	authed.PUT("/play-profiles/:id", updatePlayProfileHandler(svc))
	authed.POST("/play-profiles/:id/verify-pin", verifyPlayProfilePINHandler(svc))
	authed.DELETE("/play-profiles/:id", deletePlayProfileHandler(svc))
}

func registerAuthedSearchRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/search", searchUnifiedHandler(svc))
	authed.GET("/search/advanced", searchAdvancedHandler(svc))
	authed.GET("/search/tmdb", searchTMDbHandler(svc))
	authed.GET("/search/sites", searchSitesHandler(svc))
}

func registerAuthedSystemExtraRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/system/config", listSystemConfigHandler(svc))
	authed.GET("/settings/schema", schemaHandler(svc))
	authed.GET("/system/events/ticket", systemEventsTicketHandler(svc))
}

func registerAuthedStatsExtraRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/stats/user/:id", statsUserHandler(svc))
	authed.GET("/stats/top-users", statsTopUsersHandler(svc))
	authed.POST("/stats/play", statsPlayHandler(svc))
}

func registerAuthedSitesExtraRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/sites/:id/resource", requirePermission(svc, "can_manage_sites"), siteResourceHandler(svc))
	authed.GET("/sites/:id/userdata", requirePermission(svc, "can_manage_sites"), siteUserdataHandler(svc))
}

func registerAuthedSubscriptionExtraRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.PUT("/subscriptions/:id", requirePermission(svc, "can_manage_subscriptions"), updateSubscriptionHandler(svc))
	authed.POST("/subscriptions/:id/search", requirePermission(svc, "can_manage_subscriptions"), searchSubscriptionHandler(svc))
}

func registerAuthedPlaylistExtraRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.POST("/playlists/:id/reorder", reorderPlaylistHandler(svc))
	authed.DELETE("/playlists/:id/items/by-id/:item_id", deletePlaylistItemByIDHandler(svc))
}

func registerAuthedDLNAControlRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.POST("/dlna/:uuid/play", dlnaPlayHandler(svc))
	authed.POST("/dlna/:uuid/pause", dlnaPauseHandler(svc))
	authed.POST("/dlna/:uuid/stop", dlnaStopHandler(svc))
	authed.GET("/dlna/:uuid/status", dlnaStatusHandler(svc))
}

func registerAuthedFavoriteAndMediaActionRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/favorites", listFavoritesAliasHandler(svc))
	authed.POST("/media/:id/favorite", addMediaFavoriteHandler(svc))
	authed.DELETE("/media/:id/favorite", removeMediaFavoriteHandler(svc))
	authed.GET("/media/:id/favorite/status", getMediaFavoriteStatusHandler(svc))
	authed.POST("/media/:id/ai-scrape", requirePermission(svc, "can_rescrape"), aiScrapeMediaHandler(svc))
	authed.POST("/media/scrape/test", requirePermission(svc, "can_rescrape"), scrapeTestHandler(svc))
	authed.POST("/media/organize", requirePermission(svc, "can_manage_files"), organizeBulkHandler(svc))
}

func registerAuthedPlaybackExtraRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/playback/:id/info", playbackInfoHandler(svc))
	authed.POST("/playback/:id/progress", playbackProgressHandler(svc))
	authed.GET("/playback/:id/external-players", externalPlayersHandler(svc))
	authed.GET("/playback/:id/external-url", externalURLHandler(svc))
	authed.GET("/playback/transcode/:job_id/status", transcodeStatusHandler(svc))
}

func registerAuthedDownloadOpsRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.POST("/download/:id/pause", requirePermission(svc, "can_manage_downloads"), downloadPauseHandler(svc))
	authed.POST("/download/:id/resume", requirePermission(svc, "can_manage_downloads"), downloadResumeHandler(svc))
	authed.POST("/download/:id/organize", requirePermission(svc, "can_manage_files"), downloadOrganizeOneHandler(svc))
	authed.POST("/download/organize", requirePermission(svc, "can_manage_files"), downloadOrganizeAllHandler(svc))
	authed.POST("/download/sync", requirePermission(svc, "can_manage_downloads"), downloadSyncHandler(svc))
	authed.POST("/download/start-auto-sync", requirePermission(svc, "can_manage_downloads"), downloadAutoSyncHandler(svc))
	authed.GET("/download/tasks", requirePermission(svc, "can_manage_downloads"), downloadTasksAliasHandler(svc))
}

func registerAuthedAssistantRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/admin/assistant/sessions", listAssistantSessionsHandler(svc))
	authed.POST("/admin/assistant/sessions", createAssistantSessionHandler(svc))
	authed.GET("/admin/assistant/session/:id", getAssistantSessionHandler(svc))
	authed.DELETE("/admin/assistant/session/:id", deleteAssistantSessionHandler(svc))
	authed.POST("/admin/assistant/chat", assistantChatHandler(svc))
	authed.POST("/admin/assistant/execute", assistantExecuteHandler(svc))
	authed.POST("/admin/assistant/undo/:op_id", assistantUndoHandler(svc))
	authed.GET("/admin/assistant/history", assistantHistoryHandler(svc))
}
