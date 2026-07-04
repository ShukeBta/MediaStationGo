package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// registerEmbyRoutes 在 r 上挂双前缀（"" + "/emby"）的 Emby 兼容路由。
func registerEmbyRoutes(r *gin.Engine, jwtSecret string, svc *service.Container) {
	for _, prefix := range []string{"/emby", ""} {
		grp := r.Group(prefix)
		grp.Use(embyNoStoreHeaders())

		registerEmbyRootRoutes(grp, prefix, svc)
		registerEmbyPublicRoutes(grp, jwtSecret, svc)
		registerEmbyPublicImageRoutes(grp, svc)

		// 鉴权后端点
		auth := grp.Group("", embyAuthRequiredWithSessionFallback(jwtSecret), activeEmbyUserRequired(svc), embyRealtimeSessionActivity(svc))
		registerEmbyAuthenticatedRoutes(auth, prefix, svc)
	}
}

type embyRouteHandlerFactory func(*service.Container) gin.HandlerFunc

func embyNoStoreHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

func registerEmbyRootRoutes(grp *gin.RouterGroup, prefix string, svc *service.Container) {
	if prefix != "/emby" {
		return
	}
	grp.GET("", embyRootHandler(svc))
	grp.HEAD("", embyRootHandler(svc))
	grp.GET("/", embyRootHandler(svc))
	grp.HEAD("/", embyRootHandler(svc))
}

func registerEmbyPublicRoutes(grp *gin.RouterGroup, jwtSecret string, svc *service.Container) {
	registerEmbyPublicSystemRoutes(grp, svc)
	registerEmbyPublicSessionRoutes(grp, jwtSecret, svc)
	registerEmbyPublicClientRoutes(grp, jwtSecret, svc)
}

func registerEmbyPublicSystemRoutes(grp *gin.RouterGroup, svc *service.Container) {
	registerEmbyGetHeadRoutes(grp, svc, []string{"/System/Info/Public", "/system/info/public"}, embySystemInfoPublicHandler)
	registerEmbyGetHeadRoutes(grp, svc, []string{"/System/Info", "/system/info"}, embySystemInfoHandler)
	registerEmbyGetRoutes(grp, svc, []string{"/System/Endpoint", "/system/endpoint"}, embySystemEndpointHandler)
	registerEmbyGetHeadRoutes(grp, svc, []string{"/System/Ext/ServerDomains", "/system/ext/serverdomains"}, embyServerDomainsHandler)
	registerEmbyGetHeadRoutes(grp, svc, []string{"/System/Configuration/Public", "/system/configuration/public"}, embyPublicServerConfigurationHandler)
	registerEmbyGetHeadRoutes(grp, svc, []string{"/Startup/Configuration", "/startup/configuration"}, embyStartupConfigurationHandler)
	registerEmbyPostRoutes(grp, svc, []string{"/Startup/Complete", "/startup/complete"}, embyNoContentHandler)
	registerEmbyGetHeadRoutes(grp, svc, []string{"/QuickConnect/Enabled", "/quickconnect/enabled"}, embyQuickConnectEnabledHandler)
	for _, path := range []string{"/System/Ping", "/system/ping"} {
		grp.GET(path, embyPingHandler(svc))
		grp.HEAD(path, embyPingHandler(svc))
		grp.POST(path, embyPingHandler(svc))
	}
}

func registerEmbyPublicSessionRoutes(grp *gin.RouterGroup, jwtSecret string, svc *service.Container) {
	for _, path := range []string{
		"/Sessions/Capabilities", "/Sessions/Capabilities/Full",
		"/sessions/capabilities", "/sessions/capabilities/full",
	} {
		grp.POST(path, embySessionCapabilitiesHandler(svc, jwtSecret))
	}

	// 30/min per IP: many Emby clients sit behind a single NAT/reverse-proxy
	// IP, so a low limit would throttle legitimate logins into 429s.
	embyLoginLimiter := middleware.NewRateLimiter(30, 1*time.Minute)
	for _, path := range []string{"/Users/AuthenticateByName", "/Users/authenticatebyname", "/users/AuthenticateByName", "/users/authenticatebyname"} {
		grp.POST(path, middleware.RateLimit(embyLoginLimiter), embyAuthByNameHandler(svc))
	}

	registerEmbyGetRoutes(grp, svc, []string{"/Users/Public", "/users/public"}, embyPublicUsersHandler)
}

func registerEmbyPublicClientRoutes(grp *gin.RouterGroup, jwtSecret string, svc *service.Container) {
	registerEmbyGetRoutes(grp, svc, []string{"/Branding/Configuration", "/branding/configuration"}, embyBrandingConfigHandler)
	registerEmbyGetHeadRoutes(grp, svc, []string{"/Branding/Css", "/branding/css"}, embyBrandingCSSHandler)
	registerEmbyGetRoutes(grp, svc, []string{"/Localization/Options", "/localization/options"}, embyLocalizationOptionsHandler)
	registerEmbyGetRoutes(grp, svc, []string{"/Localization/Cultures", "/Localization/cultures", "/localization/cultures"}, embyLocalizationCulturesHandler)
	registerEmbyGetHeadRoutes(grp, svc, []string{"/CustomCssJS/Scripts", "/customcssjs/scripts"}, embyCustomCSSJSScriptsHandler)
	for _, path := range []string{"/embywebsocket", "/EmbyWebSocket"} {
		grp.GET(path, embyWebSocketHandler(svc, jwtSecret))
		grp.HEAD(path, embyNoContentHandler(svc))
	}
	for _, path := range []string{"/Sessions/Logout", "/sessions/logout"} {
		grp.POST(path, embySessionLogoutHandler(svc, jwtSecret))
	}
	grp.GET("/DisplayPreferences/:id", embyDisplayPreferencesHandler(svc))
	grp.POST("/DisplayPreferences/:id", embySaveDisplayPreferencesHandler(svc))
	grp.GET("/displaypreferences/:id", embyDisplayPreferencesHandler(svc))
	grp.POST("/displaypreferences/:id", embySaveDisplayPreferencesHandler(svc))
}

func registerEmbyPublicImageRoutes(grp *gin.RouterGroup, svc *service.Container) {
	// 图片公开（Infuse 缓存 URL 时会丢 token）
	grp.GET("/Items/:id/Images/:type", embyItemImageHandler(svc))
	grp.GET("/Items/:id/Images/:type/:index", embyItemImageHandler(svc))
	grp.HEAD("/Items/:id/Images/:type", embyItemImageHandler(svc))
	grp.GET("/items/:id/images/:type", embyItemImageHandler(svc))
	grp.GET("/items/:id/images/:type/:index", embyItemImageHandler(svc))
	grp.HEAD("/items/:id/images/:type", embyItemImageHandler(svc))
}

func registerEmbyGetRoutes(grp *gin.RouterGroup, svc *service.Container, paths []string, factory embyRouteHandlerFactory) {
	for _, path := range paths {
		grp.GET(path, factory(svc))
	}
}

func registerEmbyGetHeadRoutes(grp *gin.RouterGroup, svc *service.Container, paths []string, factory embyRouteHandlerFactory) {
	for _, path := range paths {
		grp.GET(path, factory(svc))
		grp.HEAD(path, factory(svc))
	}
}

func registerEmbyPostRoutes(grp *gin.RouterGroup, svc *service.Container, paths []string, factory embyRouteHandlerFactory) {
	for _, path := range paths {
		grp.POST(path, factory(svc))
	}
}

func registerEmbyAuthenticatedRoutes(auth *gin.RouterGroup, prefix string, svc *service.Container) {
	registerEmbyAuthenticatedUserRoutes(auth, svc)
	registerEmbyAuthenticatedItemRoutes(auth, svc)
	registerEmbyAuthenticatedPlaybackRoutes(auth, prefix, svc)
	registerEmbyAuthenticatedProgressRoutes(auth, svc)
	registerEmbyAuthenticatedUserDataRoutes(auth, svc)
	registerEmbyAuthenticatedSystemRoutes(auth, svc)
	registerLowercaseEmbyAuthRoutes(auth, svc)
}

func registerEmbyAuthenticatedUserRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.GET("/Users/Me", embyMeHandler(svc))
	auth.GET("/Users", embyListUsersHandler(svc))
	auth.GET("/Users/:userId", embyGetUserByIDHandler(svc))
	auth.GET("/Users/:userId/Views", embyViewsHandler(svc))
	auth.GET("/Library/MediaFolders", embyViewsHandler(svc))
	auth.GET("/Library/VirtualFolders", embyVirtualFoldersHandler(svc))
	auth.GET("/Library/SelectableMediaFolders", embyVirtualFoldersHandler(svc))
}

func registerEmbyAuthenticatedItemRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.GET("/Items", embyItemsHandler(svc))
	auth.GET("/Users/:userId/Items", embyItemsHandler(svc))
	auth.GET("/Items/Counts", embyItemsCountsHandler(svc))
	auth.GET("/Users/:userId/Items/Counts", embyItemsCountsHandler(svc))
	auth.GET("/Items/Latest", embyLatestItemsHandler(svc))
	auth.GET("/Items/Resume", embyResumeItemsHandler(svc))
	auth.GET("/Items/:id", embyItemByIDHandler(svc))
	auth.GET("/Users/:userId/Items/:id", embyUserItemByIDHandler(svc))
	auth.GET("/Shows/:id/Seasons", embyShowSeasonsHandler(svc))
	auth.GET("/Shows/:id/Episodes", embyShowEpisodesHandler(svc))
	auth.GET("/Users/:userId/Shows/:id/Seasons", embyShowSeasonsHandler(svc))
	auth.GET("/Users/:userId/Shows/:id/Episodes", embyShowEpisodesHandler(svc))
	auth.GET("/Shows/NextUp", embyEmptyItemsHandler(svc))
	auth.GET("/Users/:userId/Shows/NextUp", embyEmptyItemsHandler(svc))
	auth.GET("/MediaSegments/:id", embyEmptyItemsHandler(svc))
	auth.GET("/Artists", embyEmptyItemsHandler(svc))
	auth.GET("/Persons", embyEmptyItemsHandler(svc))
	auth.GET("/Genres", embyEmptyItemsHandler(svc))
	auth.GET("/Shows/Upcoming", embyEmptyItemsHandler(svc))
	auth.GET("/Users/:userId/Shows/Upcoming", embyEmptyItemsHandler(svc))
	auth.GET("/Items/:id/Similar", embyEmptyItemsHandler(svc))
	auth.GET("/Items/:id/ThumbnailSet", embyEmptyItemsHandler(svc))
	auth.GET("/Items/:id/ThemeMedia", embyThemeMediaHandler(svc))
	auth.GET("/Users/:userId/Items/:id/SpecialFeatures", embyEmptyItemsHandler(svc))
	auth.GET("/Users/:userId/Items/:id/Intros", embyEmptyItemsHandler(svc))
	auth.GET("/Items/:id/SpecialFeatures", embyEmptyItemsHandler(svc))
	auth.GET("/Items/:id/Intros", embyEmptyItemsHandler(svc))
	auth.GET("/api/danmu/:id/raw", embyDanmuRawHandler(svc))
}

func registerEmbyAuthenticatedPlaybackRoutes(auth *gin.RouterGroup, prefix string, svc *service.Container) {
	auth.GET("/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
	auth.POST("/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
	auth.GET("/Users/:userId/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))
	auth.POST("/Users/:userId/Items/:id/PlaybackInfo", embyPlaybackInfoHandler(svc))

	registerEmbyVideoStreamRoutes(auth, svc, "/Videos")
	if prefix == "/emby" {
		auth.GET("/api/stream/:id", embyVideoStreamHandler(svc, service.CloudPlaybackModeSTRM))
		auth.HEAD("/api/stream/:id", embyVideoStreamHandler(svc, service.CloudPlaybackModeSTRM))
	}
	auth.GET("/Videos/:id/master.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.HEAD("/Videos/:id/master.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.GET("/Videos/:id/main.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.HEAD("/Videos/:id/main.m3u8", embyVideoHLSPlaylistHandler(svc))
	auth.GET("/Videos/:id/:seg", embyVideoHLSSegmentHandler(svc))
}

func registerEmbyVideoStreamRoutes(auth *gin.RouterGroup, svc *service.Container, basePath string) {
	streamHandler := func() gin.HandlerFunc {
		return embyVideoStreamHandler(svc, service.CloudPlaybackModeRedirectProxy)
	}
	for _, path := range []string{"/:id/stream", "/:id/stream.:container", "/:id/original", "/:id/original.:container"} {
		fullPath := basePath + path
		auth.GET(fullPath, streamHandler())
		auth.HEAD(fullPath, streamHandler())
	}
}

func registerEmbyAuthenticatedProgressRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.POST("/Sessions/Playing", embyPlayingProgressHandler(svc))
	auth.POST("/Sessions/Playing/Progress", embyPlayingProgressHandler(svc))
	auth.POST("/Sessions/Playing/Stopped", embyPlayingProgressHandler(svc))
}

func registerEmbyAuthenticatedUserDataRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.POST("/Users/:userId/FavoriteItems/:itemId", embyFavoriteHandler(svc, true))
	auth.DELETE("/Users/:userId/FavoriteItems/:itemId", embyFavoriteHandler(svc, false))
	auth.POST("/Users/:userId/PlayedItems/:itemId", embyMarkPlayedHandler(svc, true))
	auth.DELETE("/Users/:userId/PlayedItems/:itemId", embyMarkPlayedHandler(svc, false))
}

func registerEmbyAuthenticatedSystemRoutes(auth *gin.RouterGroup, svc *service.Container) {
	auth.GET("/Sessions", embySessionsHandler(svc))
	auth.GET("/System/Configuration", embyServerConfigurationHandler(svc))
	auth.GET("/System/WakeOnLanInfo", embyEmptyArrayHandler(svc))
	auth.GET("/ScheduledTasks", embyEmptyArrayHandler(svc))
	auth.GET("/LiveTv/Recordings", embyEmptyItemsHandler(svc))
	auth.GET("/System/ActivityLog/Entries", embyEmptyItemsHandler(svc))
	auth.GET("/Web/ConfigurationPages", embyEmptyArrayHandler(svc))
	auth.POST("/Users/:userId/Configuration", embyNoContentHandler(svc))
}
