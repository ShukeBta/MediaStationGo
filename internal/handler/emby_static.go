package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func embyNoContentHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	}
}

func embyWebSocketHandler(svc *service.Container, jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		recordEmbyPublicSessionActivity(c, svc, jwtSecret)
		if !websocket.IsWebSocketUpgrade(c.Request) {
			c.Status(http.StatusNoContent)
			return
		}
		conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.NextReader(); err != nil {
					return
				}
			}
		}()

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				recordEmbyPublicSessionActivity(c, svc, jwtSecret)
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}
}

func embyServerConfigurationHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"EnableFolderView":                   true,
			"EnableGroupingIntoCollections":      true,
			"EnableExternalContentInSuggestions": false,
			"ImageSavingConvention":              "Compatible",
		})
	}
}

func embyPublicServerConfigurationHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"IsStartupWizardCompleted": true,
			"EnableRemoteAccess":       true,
			"EnableUPnP":               false,
			"EnableHttps":              false,
			"RequireHttps":             false,
			"LocalNetworkSubnets":      []string{},
			"LocalNetworkAddresses":    []string{},
			"RemoteClientBitrateLimit": 0,
		})
	}
}

func embyStartupConfigurationHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"IsStartupWizardCompleted":  true,
			"StartupWizardCompleted":    true,
			"EnableRemoteAccess":        true,
			"UICulture":                 "zh-CN",
			"MetadataCountryCode":       "CN",
			"PreferredMetadataLanguage": "zh-CN",
		})
	}
}

func embyQuickConnectEnabledHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, false)
	}
}

func embyEmptyItemsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"Items": []any{}, "TotalRecordCount": 0})
	}
}

func embyEmptyArrayHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []any{})
	}
}

func embyCustomCSSJSScriptsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "application/javascript; charset=utf-8", nil)
	}
}

func embyLocalizationCulturesHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []gin.H{
			{
				"DisplayName":                 "简体中文",
				"Name":                        "zh-CN",
				"ThreeLetterISOLanguageName":  "zho",
				"TwoLetterISOLanguageName":    "zh",
				"ThreeLetterISOLanguageNames": []string{"zho", "chi"},
				"IsRightToLeft":               false,
			},
			{
				"DisplayName":                 "English",
				"Name":                        "en-US",
				"ThreeLetterISOLanguageName":  "eng",
				"TwoLetterISOLanguageName":    "en",
				"ThreeLetterISOLanguageNames": []string{"eng"},
				"IsRightToLeft":               false,
			},
		})
	}
}

func embyThemeMediaHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		empty := gin.H{"Items": []any{}, "TotalRecordCount": 0}
		c.JSON(http.StatusOK, gin.H{
			"ThemeVideosResult":     empty,
			"ThemeSongsResult":      empty,
			"SoundtrackSongsResult": empty,
		})
	}
}

func embyServerDomainsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []any{})
	}
}

func embyDanmuRawHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain; charset=utf-8", nil)
	}
}

func embyBrandingConfigHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"LoginDisclaimer":     "",
			"CustomCss":           "",
			"SplashscreenEnabled": false,
		})
	}
}

func embyBrandingCSSHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Data(http.StatusOK, "text/css; charset=utf-8", []byte(""))
	}
}

func embyLocalizationOptionsHandler(_ *service.Container) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, []map[string]any{
			{"Name": "简体中文", "Value": "zh-CN"},
			{"Name": "English", "Value": "en-US"},
		})
	}
}
