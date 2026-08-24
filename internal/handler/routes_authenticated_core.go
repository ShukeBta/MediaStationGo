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
	authed.GET("/libraries", requirePermission(svc, "can_play_media"), listLibrariesHandler(svc))
	authed.POST("/libraries", middleware.AdminRequired(), createLibraryHandler(svc))
	authed.GET("/libraries/:id", requirePermission(svc, "can_play_media"), getLibraryHandler(svc))
	authed.PATCH("/libraries/:id", middleware.AdminRequired(), updateLibraryHandler(svc))
	authed.DELETE("/libraries/:id", middleware.AdminRequired(), deleteLibraryHandler(svc))
	authed.GET("/libraries/:id/roots", middleware.AdminRequired(), listLibraryRootsHandler(svc))
	authed.POST("/libraries/:id/roots", middleware.AdminRequired(), createLibraryRootHandler(svc))
	authed.PATCH("/libraries/:id/roots/:root_id", middleware.AdminRequired(), updateLibraryRootHandler(svc))
	authed.DELETE("/libraries/:id/roots/:root_id", middleware.AdminRequired(), deleteLibraryRootHandler(svc))
	authed.POST("/libraries/:id/roots/:root_id/scan", middleware.AdminRequired(), scanLibraryRootHandler(svc))
	authed.POST("/libraries/:id/scan", middleware.AdminRequired(), scanLibraryHandler(svc))
	authed.POST("/libraries/:id/scrape", middleware.AdminRequired(), scrapeLibraryHandler(svc))
	authed.POST("/libraries/:id/title-cleanup/preview", middleware.AdminRequired(), previewMediaTitleCleanupHandler(svc))
	authed.GET("/libraries/:id/title-cleanup/preview/:job_id", middleware.AdminRequired(), getMediaTitleCleanupJobHandler(svc))
	authed.POST("/libraries/:id/title-cleanup/apply", middleware.AdminRequired(), applyMediaTitleCleanupHandler(svc))
	authed.POST("/libraries/:id/media-aggregation", middleware.AdminRequired(), updateMediaAggregationHandler(svc))
	authed.GET("/libraries/:id/generated-artwork", middleware.AdminRequired(), generatedArtworkStatusHandler(svc))
	authed.POST("/libraries/:id/generated-artwork", middleware.AdminRequired(), runGeneratedArtworkHandler(svc))
	authed.DELETE("/libraries/:id/generated-artwork", middleware.AdminRequired(), cancelGeneratedArtworkHandler(svc))

	authed.GET("/libraries/:id/media", requirePermission(svc, "can_play_media"), listMediaHandler(svc))
	authed.GET("/libraries/:id/series", requirePermission(svc, "can_play_media"), listLibrarySeriesHandler(svc))
	authed.GET("/libraries/:id/series/episodes", requirePermission(svc, "can_play_media"), listLibrarySeriesEpisodesHandler(svc))
	authed.GET("/libraries/:id/seasons", requirePermission(svc, "can_play_media"), listSeasonsHandler(svc))
}

func registerAuthedMediaRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/media/:id", requirePermission(svc, "can_play_media"), getMediaHandler(svc))
	authed.GET("/media/:id/versions", requirePermission(svc, "can_play_media"), getMediaVersionsHandler(svc))
	authed.GET("/media/:id/parts", requirePermission(svc, "can_play_media"), getMediaPartsHandler(svc))
	authed.DELETE("/media/:id/versions/:version_id", requirePermission(svc, "can_play_media"), deleteMediaVersionHandler(svc))
	authed.GET("/media", requirePermission(svc, "can_play_media"), searchMediaHandler(svc))
	authed.PATCH("/media/:id/metadata", middleware.AdminRequired(), updateMediaMetadataHandler(svc))
	authed.POST("/media/:id/scrape", middleware.AdminRequired(), scrapeOneHandler(svc))
	authed.GET("/media/:id/scrape/search", middleware.AdminRequired(), manualScrapeSearchHandler(svc))
	authed.POST("/media/:id/scrape/apply", middleware.AdminRequired(), manualScrapeApplyOneHandler(svc))
	authed.POST("/media/scrape/apply", middleware.AdminRequired(), manualScrapeApplyBatchHandler(svc))
	authed.POST("/media/probes/missing", middleware.AdminRequired(), probeMissingMediaHandler(svc))
	authed.POST("/media/:id/probe", middleware.AdminRequired(), reprobeHandler(svc))
	authed.POST("/media/:id/generated-artwork", middleware.AdminRequired(), generateMediaArtworkAtHandler(svc))
	authed.DELETE("/media/:id", middleware.AdminRequired(), deleteMediaHandler(svc))
	authed.POST("/media/:id/restore", restoreMediaHandler(svc))
	authed.DELETE("/media/:id/purge", purgeMediaHandler(svc))
	authed.GET("/media/:id/subtitles", requirePermission(svc, "can_play_media"), listSubtitlesHandler(svc))
	authed.GET("/subtitles/:id", requirePermission(svc, "can_play_media"), serveSubtitleHandler(svc))
	authed.DELETE("/media/:id/subtitles", middleware.AdminRequired(), deleteSubtitleHandler(svc))
	authed.POST("/media/:id/subtitles/cloud-refresh", middleware.AdminRequired(), refreshCloudSubtitlesHandler(svc))
	authed.POST("/media/:id/subtitles/search", middleware.AdminRequired(), searchSubtitleCandidatesHandler(svc))
	authed.POST("/media/:id/subtitles/season/search", middleware.AdminRequired(), searchSeasonSubtitleCandidatesHandler(svc))
	authed.POST("/media/:id/subtitles/season/apply", middleware.AdminRequired(), applySeasonSubtitleCandidatesHandler(svc))
	authed.GET("/media/:id/subtitles/season/tasks/:task_id", middleware.AdminRequired(), getSeasonSubtitleTaskHandler(svc))
	authed.POST("/media/:id/subtitles/season/tasks/:task_id/retry", middleware.AdminRequired(), retrySeasonSubtitleTaskHandler(svc))
	authed.POST("/media/:id/subtitles/preview", middleware.AdminRequired(), previewSubtitleCandidateHandler(svc))
	authed.POST("/media/:id/subtitles/apply", middleware.AdminRequired(), applySubtitleCandidateHandler(svc))
	authed.POST("/media/:id/subtitles/asr", middleware.AdminRequired(), createSubtitleASRHandler(svc))
	authed.GET("/media/:id/subtitles/asr/:task_id", middleware.AdminRequired(), getSubtitleASRHandler(svc))
	authed.GET("/subtitles/asr/tasks", middleware.AdminRequired(), listSubtitleASRTasksHandler(svc))
	authed.GET("/subtitles/asr/profiles", middleware.AdminRequired(), listSubtitleASRProfilesHandler(svc))
	authed.GET("/subtitles/asr/models", middleware.AdminRequired(), listSubtitleASRModelsHandler(svc))
	authed.POST("/subtitles/asr/tasks/:task_id/retry", middleware.AdminRequired(), retrySubtitleASRTaskHandler(svc))
	authed.POST("/subtitles/asr/tasks/:task_id/model", middleware.AdminRequired(), updateSubtitleASRTaskModelHandler(svc))
	authed.POST("/subtitles/asr/tasks/:task_id/cancel", middleware.AdminRequired(), cancelSubtitleASRTaskHandler(svc))
	authed.POST("/subtitles/asr/tasks/:task_id/retranslate", middleware.AdminRequired(), retranslateSubtitleASRTaskHandler(svc))
	authed.DELETE("/subtitles/asr/tasks/:task_id", middleware.AdminRequired(), deleteSubtitleASRTaskHandler(svc))
	authed.POST("/subtitles/cloud-cache/invalidate", middleware.AdminRequired(), invalidateCloudSubtitleCacheHandler(svc))
	authed.POST("/subtitles/cloud-cache/backfill", middleware.AdminRequired(), backfillCloudSubtitlesHandler(svc))
	authed.POST("/media/:id/nfo", middleware.AdminRequired(), exportNFOHandler(svc))
	authed.POST("/libraries/:id/nfo", middleware.AdminRequired(), exportLibraryNFOHandler(svc))
}

func registerAuthedPlaybackAndProxyRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/pipeline/media/:id/asr-audio", middleware.AdminRequired(), pipelineASRAudioHandler(svc))
	authed.POST("/pipeline/subtitles/translate", middleware.AdminRequired(), pipelineTranslateSubtitlesHandler(svc))
	authed.GET("/stream/:id", requirePermission(svc, "can_play_media"), streamHandler(svc))
	authed.HEAD("/stream/:id", requirePermission(svc, "can_play_media"), streamHandler(svc))
	authed.GET("/hls/:id/index.m3u8", requirePermission(svc, "can_play_media"), hlsPlaylistHandler(svc))
	authed.GET("/hls/:id/:seg", requirePermission(svc, "can_play_media"), hlsSegmentHandler(svc))
	authed.DELETE("/hls/:id", requirePermission(svc, "can_play_media"), stopTranscodeHandler(svc))

	authed.GET("/cloud/play/:type", requirePermission(svc, "can_play_media"), cloudPlayHandler(svc))
	authed.HEAD("/cloud/play/:type", requirePermission(svc, "can_play_media"), cloudPlayHandler(svc))

	authed.GET("/img/cloud/:type", cloudArtworkProxyHandler(svc))
	authed.HEAD("/img/cloud/:type", cloudArtworkProxyHandler(svc))
	authed.GET("/img", imageProxyHandler(svc))
}

func registerAuthedCollectionRoutes(authed *gin.RouterGroup, svc *service.Container) {
	authed.GET("/history", requirePermission(svc, "can_view_history"), recentHistoryHandler(svc))
	authed.POST("/history", requirePermission(svc, "can_play_media"), recordProgressHandler(svc))

	authed.GET("/favourites", requirePermission(svc, "can_favorite"), listFavouritesHandler(svc))
	authed.POST("/favourites/:id", requirePermission(svc, "can_favorite"), toggleFavouriteHandler(svc))

	authed.GET("/storage", storageBreakdownHandler(svc))

	authed.GET("/playlists", requirePermission(svc, "can_play_media"), listPlaylistsHandler(svc))
	authed.POST("/playlists", requirePermission(svc, "can_play_media"), createPlaylistHandler(svc))
	authed.GET("/playlists/:id", requirePermission(svc, "can_play_media"), getPlaylistHandler(svc))
	authed.POST("/playlists/:id/items", requirePermission(svc, "can_play_media"), addPlaylistItemHandler(svc))
	authed.DELETE("/playlists/:id/items/:media_id", requirePermission(svc, "can_play_media"), removePlaylistItemHandler(svc))
	authed.DELETE("/playlists/:id", requirePermission(svc, "can_play_media"), deletePlaylistHandler(svc))
}
