package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerAuthedDownloadRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/downloads", requirePermission(svc, "can_manage_downloads"), listDownloadsHandler(svc))
	authed.POST("/downloads", requirePermission(svc, "can_manage_downloads"), addDownloadHandler(svc))
	authed.DELETE("/downloads/:hash", requirePermission(svc, "can_manage_downloads"), deleteDownloadHandler(svc))
	authed.POST("/downloads/relocate", requirePermission(svc, "can_manage_downloads"), relocateDownloadHandler(svc))
	authed.POST("/downloads/reload", requirePermission(svc, "can_manage_downloads"), reloadDownloadConfigHandler(svc))
}

func registerAuthedSubscriptionRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/subscriptions", middleware.AdminRequired(), listSubscriptionsHandler(svc))
	authed.GET("/subscriptions/history", middleware.AdminRequired(), listSubscriptionHistoryHandler(svc))
	authed.POST("/subscriptions", middleware.AdminRequired(), createSubscriptionHandler(svc))
	authed.DELETE("/subscriptions/:id", middleware.AdminRequired(), deleteSubscriptionHandler(svc))
	authed.DELETE("/subscriptions/:id/history", middleware.AdminRequired(), purgeSubscriptionHistoryHandler(svc))
	authed.POST("/subscriptions/:id/restore", middleware.AdminRequired(), restoreSubscriptionHandler(svc))
	authed.POST("/subscriptions/:id/run", middleware.AdminRequired(), runSubscriptionHandler(svc))
}

func registerAuthedStatsDiscoveryAndAIRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/stats", requirePermission(svc, "can_view_dashboard"), statsHandler(svc))
	authed.GET("/tasks", middleware.AdminRequired(), tasksHandler(svc))
	registerTaskHistoryRoutes(authed, svc)

	authed.GET("/discover/trending", requirePermission(svc, "can_view_discover"), trendingHandler(svc))
	authed.GET("/discover/popular", requirePermission(svc, "can_view_discover"), popularHandler(svc))

	authed.GET("/ai/status", requirePermission(svc, "can_use_ai"), aiStatusHandler(svc))
	authed.POST("/ai/search", requirePermission(svc, "can_use_ai"), smartSearchHandler(svc))
	authed.GET("/ai/recommend", requirePermission(svc, "can_use_ai"), aiRecommendHandler(svc))
}

func registerAuthedFileRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/files", middleware.AdminRequired(), browseFilesHandler(svc))
	authed.POST("/files/folders", middleware.AdminRequired(), createFolderHandler(svc))
	authed.PUT("/files/rename", middleware.AdminRequired(), renameFileHandler(svc))
	authed.DELETE("/files", middleware.AdminRequired(), deleteFileHandler(svc))
	authed.POST("/files/transfer", middleware.AdminRequired(), transferFileHandler(svc))
}

func registerAuthedDLNARoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/dlna/devices", requirePermission(svc, "can_cast"), dlnaListHandler(svc))
	authed.POST("/dlna/cast", requirePermission(svc, "can_cast"), dlnaCastHandler(svc))
}

func registerAuthedSTRMRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.PUT("/media/:id/strm", middleware.AdminRequired(), setSTRMHandler(svc))
	authed.DELETE("/media/:id/strm", middleware.AdminRequired(), clearSTRMHandler(svc))
	authed.GET("/strm/output-presets", middleware.AdminRequired(), listSTRMOutputPresetsHandler(svc))
	authed.POST("/strm/import", middleware.AdminRequired(), importSTRMHandler(svc))
	authed.POST("/strm/generate", middleware.AdminRequired(), generateSTRMHandler(svc))
	authed.POST("/strm/generate-from-tree", middleware.AdminRequired(), generateSTRMFromTreeHandler(svc))
	authed.POST("/strm/repair", middleware.AdminRequired(), repairSTRMHandler(svc))
}

func registerAuthedDuplicateRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/duplicates", middleware.AdminRequired(), listDuplicatesHandler(svc))
	authed.POST("/duplicates/scan", middleware.AdminRequired(), detectDuplicatesHandler(svc))
	authed.POST("/duplicates/unmark", middleware.AdminRequired(), unmarkDuplicatesHandler(svc))
}

func registerAuthedSiteRoutes(authed *gin.RouterGroup, svc *service.Container) {
	siteHandler := NewSiteHandler(svc)
	authed.GET("/sites", requirePermission(svc, "can_manage_sites"), siteHandler.ListSites)
	authed.GET("/sites/types", requirePermission(svc, "can_manage_sites"), siteHandler.GetSiteTypes)
	authed.GET("/sites/auth-types", requirePermission(svc, "can_manage_sites"), siteHandler.GetAuthTypes)
	authed.GET("/sites/search", requirePermission(svc, "can_manage_sites"), siteSearchHandler(svc))
	authed.GET("/sites/categories", requirePermission(svc, "can_manage_sites"), siteCategoriesHandler(svc))
	authed.GET("/sites/browse", requirePermission(svc, "can_manage_sites"), siteBrowseHandler(svc))
	authed.GET("/sites/detail", requirePermission(svc, "can_manage_sites"), siteDetailHandler(svc))
	authed.POST("/sites/download", requirePermission(svc, "can_manage_sites"), siteDownloadHandler(svc))
	authed.POST("/sites/download/prepare", requirePermission(svc, "can_manage_sites"), siteDownloadPrepareHandler(svc))
	authed.POST("/sites/download/confirm", requirePermission(svc, "can_manage_sites"), siteDownloadConfirmHandler(svc))
	authed.POST("/sites/download/cancel", requirePermission(svc, "can_manage_sites"), siteDownloadCancelHandler(svc))
	authed.POST("/sites/subscribe", requirePermission(svc, "can_manage_sites"), siteSubscribeHandler(svc))
	authed.POST("/sites", requirePermission(svc, "can_manage_sites"), siteHandler.CreateSite)
	authed.GET("/sites/:id", requirePermission(svc, "can_manage_sites"), siteHandler.GetSite)
	authed.PUT("/sites/:id", requirePermission(svc, "can_manage_sites"), siteHandler.UpdateSite)
	authed.DELETE("/sites/:id", requirePermission(svc, "can_manage_sites"), siteHandler.DeleteSite)
	authed.POST("/sites/:id/test", requirePermission(svc, "can_manage_sites"), siteHandler.TestSite)
}

func registerAuthedRecycleAndRealtimeRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/recycle", requirePermission(svc, "can_manage_files"), listRecycleHandler(svc))
	authed.POST("/recycle/soft-delete", middleware.AdminRequired(), softDeleteMediaBatchHandler(svc))
	authed.POST("/recycle/restore", requirePermission(svc, "can_manage_files"), restoreMediaBatchHandler(svc))
	authed.POST("/recycle/purge", requirePermission(svc, "can_manage_files"), purgeMediaBatchHandler(svc))

	authed.GET("/ws", wsHandler(svc))
	authed.GET("/events", sseHandler(svc))
}

func registerAuthedSchedulerRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/tasks/startup", taskStartupHandler(svc))
	authed.GET("/scheduler/tasks", schedulerListTasksHandler(svc))
	authed.POST("/scheduler/tasks/:id/run", middleware.AdminRequired(), schedulerRunTaskHandler(svc))
	authed.GET("/scheduler/status", schedulerGetStatusHandler(svc))
}
