package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerLowercaseEmbyAuthRoutes(auth *gin.RouterGroup, svc *service.Container) {
	registerLowercaseEmbyUserRoutes(auth, svc)
	registerLowercaseEmbyItemRoutes(auth, svc)
	registerLowercaseEmbyPlaybackRoutes(auth, svc)
	registerLowercaseEmbyProgressRoutes(auth, svc)
	registerLowercaseEmbyUserDataRoutes(auth, svc)
	registerLowercaseEmbySystemRoutes(auth, svc)
}

func registerLowercaseEmbyUserRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.GET("/users/me", embyMeHandler(svc))
	auth.GET("/users", embyListUsersHandler(svc))
	auth.GET("/users/:userId", embyGetUserByIDHandler(svc))
	auth.GET("/users/:userId/views", embyViewsHandler(svc))
	auth.GET("/library/mediafolders", embyViewsHandler(svc))
	auth.GET("/library/virtualfolders", embyVirtualFoldersHandler(svc))
	auth.GET("/library/selectablemediafolders", embyVirtualFoldersHandler(svc))
}

func registerLowercaseEmbyItemRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.GET("/items", embyItemsHandler(svc))
	auth.GET("/users/:userId/items", embyItemsHandler(svc))
	auth.GET("/items/counts", embyItemsCountsHandler(svc))
	auth.GET("/users/:userId/items/counts", embyItemsCountsHandler(svc))
	auth.GET("/items/latest", embyLatestItemsHandler(svc))
	auth.GET("/items/resume", embyResumeItemsHandler(svc))
	auth.GET("/items/:id", embyItemByIDHandler(svc))
	auth.GET("/users/:userId/items/:id", embyUserItemByIDHandler(svc))
	auth.GET("/shows/:id/seasons", embyShowSeasonsHandler(svc))
	auth.GET("/shows/:id/episodes", embyShowEpisodesHandler(svc))
	auth.GET("/users/:userId/shows/:id/seasons", embyShowSeasonsHandler(svc))
	auth.GET("/users/:userId/shows/:id/episodes", embyShowEpisodesHandler(svc))
	auth.GET("/shows/nextup", embyEmptyItemsHandler(svc))
	auth.GET("/users/:userId/shows/nextup", embyEmptyItemsHandler(svc))
	auth.GET("/mediasegments/:id", embyEmptyItemsHandler(svc))
	auth.GET("/artists", embyEmptyItemsHandler(svc))
	auth.GET("/persons", embyEmptyItemsHandler(svc))
	auth.GET("/genres", embyEmptyItemsHandler(svc))
	auth.GET("/shows/upcoming", embyEmptyItemsHandler(svc))
	auth.GET("/users/:userId/shows/upcoming", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/similar", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/thumbnailset", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/thememedia", embyThemeMediaHandler(svc))
	auth.GET("/users/:userId/items/:id/specialfeatures", embyEmptyItemsHandler(svc))
	auth.GET("/users/:userId/items/:id/intros", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/specialfeatures", embyEmptyItemsHandler(svc))
	auth.GET("/items/:id/intros", embyEmptyItemsHandler(svc))
}

func registerLowercaseEmbyPlaybackRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.GET("/items/:id/playbackinfo", embyPlaybackInfoHandler(svc))
	auth.POST("/items/:id/playbackinfo", embyPlaybackInfoHandler(svc))
	auth.GET("/users/:userId/items/:id/playbackinfo", embyPlaybackInfoHandler(svc))
	auth.POST("/users/:userId/items/:id/playbackinfo", embyPlaybackInfoHandler(svc))

	registerEmbyVideoStreamRoutes(auth, svc, "/videos")
	auth.GET("/videos/:id/master.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.HEAD("/videos/:id/master.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.GET("/videos/:id/main.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.HEAD("/videos/:id/main.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.GET("/videos/:id/:seg", embyVideoHLSSegmentHandler(svc))
}

func registerLowercaseEmbyProgressRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.POST("/sessions/playing", embyPlayingProgressHandler(svc))
	auth.POST("/sessions/playing/progress", embyPlayingProgressHandler(svc))
	auth.POST("/sessions/playing/stopped", embyPlayingProgressHandler(svc))
}

func registerLowercaseEmbyUserDataRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.POST("/users/:userId/favoriteitems/:itemId", embyFavoriteHandler(svc, true))
	auth.DELETE("/users/:userId/favoriteitems/:itemId", embyFavoriteHandler(svc, false))
	auth.POST("/users/:userId/playeditems/:itemId", embyMarkPlayedHandler(svc, true))
	auth.DELETE("/users/:userId/playeditems/:itemId", embyMarkPlayedHandler(svc, false))
}

func registerLowercaseEmbySystemRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.GET("/sessions", embySessionsHandler(svc))
	auth.GET("/system/configuration", embyServerConfigurationHandler(svc))
	auth.GET("/system/wakeonlaninfo", embyEmptyArrayHandler(svc))
	auth.GET("/scheduledtasks", embyEmptyArrayHandler(svc))
	auth.GET("/livetv/recordings", embyEmptyItemsHandler(svc))
	auth.GET("/system/activitylog/entries", embyEmptyItemsHandler(svc))
	auth.GET("/web/configurationpages", embyEmptyArrayHandler(svc))
	auth.POST("/users/:userId/configuration", embyNoContentHandler(svc))
}
