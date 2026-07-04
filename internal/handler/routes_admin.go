// Package handler — admin-only routes.
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerAdminRoutes(api *gin.RouterGroup, cfg *config.Config, svc *service.Container) {
	admin := api.Group("/admin")
	admin.Use(middleware.AuthRequired(cfg.Secrets.JWTSecret), middleware.AdminRequired())
	registerAdminUserRoutes(admin, svc)
	registerAdminPermissionRoutes(admin, svc)
	registerAdminStorageRoutes(admin, svc)
	registerAdminCloudRoutes(admin, svc)
	registerAdminDownloadClientRoutes(admin, svc)
	registerAdminSystemRoutes(admin, svc)
	registerAdminBackupRoutes(admin, svc)
	registerAdminNotificationRoutes(admin, svc)
	registerAdminTelegramRoutes(admin, svc)
	registerAdminOrganizerRoutes(admin, svc)
	registerAdminRepairRoutes(admin, svc)
	registerAdminAPIConfigRoutes(admin, svc)
	registerAdminSchedulerRoutes(admin, svc)
	registerAdminRecognitionWordRoutes(admin, svc)
}

func registerAdminUserRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/users", listUsersHandler(svc))
	admin.POST("/users", createUserHandler(svc))
	admin.PATCH("/users/:id", updateUserHandler(svc))
	admin.PATCH("/users/:id/password", resetUserPasswordHandler(svc))
	admin.PATCH("/users/:id/status", updateUserStatusHandler(svc))
	admin.PATCH("/users/:id/role", adminUpdateRoleHandler(svc))
	admin.DELETE("/users/:id", deleteUserHandler(svc))
	admin.GET("/settings", listSettingsHandler(svc))
	admin.PUT("/settings", updateSettingHandler(svc))
	admin.GET("/logs", recentLogsHandler(svc))
}

func registerAdminPermissionRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/users/:id/permissions", getUserPermissionsHandler(svc))
	admin.PUT("/users/:id/permissions", updateUserPermissionsHandler(svc))
	admin.POST("/users/:id/permissions/reset", resetUserPermissionsHandler(svc))
}

func registerAdminStorageRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/storage/status", listStorageConfigsHandler(svc))
	admin.GET("/storage/:type", getStorageConfigHandler(svc))
	admin.PUT("/storage/:type", saveStorageConfigHandler(svc))
	admin.POST("/storage/:type/test", testStorageConfigHandler(svc))
	admin.POST("/storage/:type/logout", logoutStorageConfigHandler(svc))
	admin.POST("/storage/:type/upload-local", storageUploadLocalHandler(svc))
}

func registerAdminCloudRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.POST("/cloud/scan-all", cloudScanAllHandler(svc))
	admin.POST("/cloud/scan/cancel", cloudScanCancelHandler(svc))
	admin.GET("/cloud/scan/status", cloudScanStatusHandler(svc))
	admin.GET("/cloud/:type/list", cloudListHandler(svc))
	admin.POST("/cloud/:type/mkdir", cloudMkdirHandler(svc))
	admin.PUT("/cloud/:type/rename", cloudRenameHandler(svc))
	admin.POST("/cloud/:type/import", cloudImportHandler(svc))
	admin.POST("/cloud/:type/mount", cloudMountHandler(svc))
	admin.POST("/cloud/:type/qr/start", cloud115QRStartHandler(svc))
	admin.POST("/cloud/:type/qr/poll", cloud115QRPollHandler(svc))
}

func registerAdminDownloadClientRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/download/clients", listDownloadClientsHandler(svc))
	admin.POST("/download/clients", createDownloadClientHandler(svc))
	admin.PUT("/download/clients/:id", updateDownloadClientHandler(svc))
	admin.DELETE("/download/clients/:id", deleteDownloadClientHandler(svc))
	admin.POST("/download/clients/:id/test", testDownloadClientHandler(svc))
	admin.GET("/download/aria2/stats", aria2StatsHandler(svc))
}

func registerAdminSystemRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.POST("/system/scheduler/:name/trigger", schedulerTriggerHandler(svc))
	admin.GET("/system/update", systemUpdateStatusHandler(svc))
	admin.POST("/system/update/check", systemUpdateCheckHandler(svc))
	admin.POST("/system/update/apply", systemUpdateApplyHandler(svc))
}

func registerAdminBackupRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/backups", listBackupsHandler(svc))
	admin.POST("/backups", createBackupHandler(svc))
	admin.DELETE("/backups", deleteBackupHandler(svc))
	admin.POST("/backups/restore", restoreBackupHandler(svc))
}

func registerAdminNotificationRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.POST("/notify/test", notifyTestHandler(svc))
	admin.GET("/notify/channels", listNotifyChannelsHandler(svc))
	admin.POST("/notify/channels", createNotifyChannelHandler(svc))
	admin.PUT("/notify/channels/:id", updateNotifyChannelHandler(svc))
	admin.DELETE("/notify/channels/:id", deleteNotifyChannelHandler(svc))
	admin.POST("/notify/channels/:id/test", testNotifyChannelHandler(svc))
}

func registerAdminTelegramRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/telegram/webhook", telegramGetWebhookHandler(svc))
	admin.POST("/telegram/webhook", telegramSetWebhookHandler(svc))
	admin.POST("/telegram/polling/start", telegramStartPollingHandler(svc))
	admin.POST("/telegram/polling/stop", telegramStopPollingHandler(svc))
}

func registerAdminOrganizerRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.POST("/media/:id/organize", organizeMediaHandler(svc))
	admin.POST("/libraries/:id/organize", organizeLibraryHandler(svc))
	admin.GET("/organize/sources", organizeSourcesHandler(svc))
	admin.POST("/organize/source", organizeDirectoryHandler(svc))
}

func registerAdminRepairRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.POST("/media/repair-rescrape", repairAndRescrapeAllHandler(svc))
	admin.POST("/libraries/:id/repair-rescrape", repairAndRescrapeLibraryHandler(svc))
}

func registerAdminAPIConfigRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/api-configs", listAPIConfigsHandler(svc))
	admin.GET("/api-configs/:provider", getAPIConfigHandler(svc))
	admin.PUT("/api-configs/:provider", updateAPIConfigHandler(svc))
	admin.DELETE("/api-configs/:provider", deleteAPIConfigHandler(svc))
}

func registerAdminSchedulerRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/scheduler", schedulerStatusHandler(svc))
	admin.POST("/scheduler/:name/run", schedulerRunHandler(svc))
}

func registerAdminRecognitionWordRoutes(admin *gin.RouterGroup, svc *service.Container) {
	admin.GET("/recognition-words", getRecognitionWordsHandler(svc))
	admin.PUT("/recognition-words", saveRecognitionWordsHandler(svc))
	admin.POST("/recognition-words/sync", syncRecognitionWordsHandler(svc))
	admin.POST("/recognition-words/test", testRecognitionWordsHandler(svc))
}
