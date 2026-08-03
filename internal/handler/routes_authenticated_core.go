package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func registerAuthedUserAndLicenseRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/me", meHandler(svc))
	authed.PATCH("/me", updateProfileHandler(svc))
	authed.POST("/me/password", changePasswordHandler(svc))
	authed.POST("/me/logout", logoutHandler(svc))

	authed.GET("/auth/permissions", getMyPermissionsHandler(svc))

	authed.GET("/license/status", middleware.AdminRequired(), licenseStatusHandler(svc))
	authed.POST("/license/activate", middleware.AdminRequired(), licenseActivateHandler(svc))
	authed.POST("/license/heartbeat", middleware.AdminRequired(), licenseHeartbeatHandler(svc))
}

func registerAuthedLibraryRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/libraries", listLibrariesHandler(svc))
	authed.POST("/libraries", middleware.AdminRequired(), createLibraryHandler(svc))
	authed.GET("/libraries/:id", getLibraryHandler(svc))
	authed.PATCH("/libraries/:id", middleware.AdminRequired(), updateLibraryHandler(svc))
	authed.DELETE("/libraries/:id", middleware.AdminRequired(), deleteLibraryHandler(svc))
	authed.GET("/libraries/:id/roots", middleware.AdminRequired(), listLibraryRootsHandler(svc))
	authed.POST("/libraries/:id/roots", middleware.AdminRequired(), createLibraryRootHandler(svc))
	authed.PATCH("/libraries/:id/roots/:root_id", middleware.AdminRequired(), updateLibraryRootHandler(svc))
	authed.DELETE("/libraries/:id/roots/:root_id", middleware.AdminRequired(), deleteLibraryRootHandler(svc))
	authed.POST("/libraries/:id/roots/:root_id/scan", middleware.AdminRequired(), scanLibraryRootHandler(svc))
	authed.POST("/libraries/:id/scan", middleware.AdminRequired(), scanLibraryHandler(svc))
	authed.POST("/libraries/:id/scrape", middleware.AdminRequired(), scrapeLibraryHandler(svc))

	authed.GET("/libraries/:id/media", listMediaHandler(svc))
	authed.GET("/libraries/:id/series", listLibrarySeriesHandler(svc))
	authed.GET("/libraries/:id/series/episodes", listLibrarySeriesEpisodesHandler(svc))
	authed.GET("/libraries/:id/seasons", listSeasonsHandler(svc))
}

func registerAuthedMediaRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/media/:id", getMediaHandler(svc))
	authed.GET("/media", searchMediaHandler(svc))
	authed.PATCH("/media/:id/metadata", middleware.AdminRequired(), updateMediaMetadataHandler(svc))
	authed.POST("/media/:id/scrape", middleware.AdminRequired(), scrapeOneHandler(svc))
	authed.GET("/media/:id/scrape/search", middleware.AdminRequired(), manualScrapeSearchHandler(svc))
	authed.POST("/media/:id/scrape/apply", middleware.AdminRequired(), manualScrapeApplyOneHandler(svc))
	authed.POST("/media/scrape/apply", middleware.AdminRequired(), manualScrapeApplyBatchHandler(svc))
	authed.POST("/media/:id/probe", middleware.AdminRequired(), reprobeHandler(svc))
	authed.DELETE("/media/:id", middleware.AdminRequired(), deleteMediaHandler(svc))
	authed.POST("/media/:id/restore", middleware.AdminRequired(), restoreMediaHandler(svc))
	authed.DELETE("/media/:id/purge", middleware.AdminRequired(), purgeMediaHandler(svc))
	authed.GET("/media/:id/subtitles", listSubtitlesHandler(svc))
	authed.GET("/subtitles/:id", serveSubtitleHandler(svc))
	authed.POST("/media/:id/nfo", middleware.AdminRequired(), exportNFOHandler(svc))
	authed.POST("/libraries/:id/nfo", middleware.AdminRequired(), exportLibraryNFOHandler(svc))
}

func registerAuthedPlaybackAndProxyRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/stream/:id", streamHandler(svc))
	authed.HEAD("/stream/:id", streamHandler(svc))
	authed.GET("/hls/:id/index.m3u8", hlsPlaylistHandler(svc))
	authed.GET("/hls/:id/:seg", hlsSegmentHandler(svc))
	authed.DELETE("/hls/:id", stopTranscodeHandler(svc))

	authed.GET("/cloud/play/:type", cloudPlayHandler(svc))
	authed.HEAD("/cloud/play/:type", cloudPlayHandler(svc))

	authed.GET("/img/cloud/:type", cloudArtworkProxyHandler(svc))
	authed.HEAD("/img/cloud/:type", cloudArtworkProxyHandler(svc))
	authed.GET("/img", imageProxyHandler(svc))
}

func registerAuthedCollectionRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/history", recentHistoryHandler(svc))
	authed.POST("/history", recordProgressHandler(svc))

	authed.GET("/favourites", listFavouritesHandler(svc))
	authed.POST("/favourites/:id", toggleFavouriteHandler(svc))

	authed.GET("/storage", storageBreakdownHandler(svc))

	authed.GET("/playlists", listPlaylistsHandler(svc))
	authed.POST("/playlists", createPlaylistHandler(svc))
	authed.GET("/playlists/:id", getPlaylistHandler(svc))
	authed.POST("/playlists/:id/items", addPlaylistItemHandler(svc))
	authed.DELETE("/playlists/:id/items/:media_id", removePlaylistItemHandler(svc))
	authed.DELETE("/playlists/:id", deletePlaylistHandler(svc))
}
