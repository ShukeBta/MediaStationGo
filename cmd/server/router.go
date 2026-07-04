package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/handler"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func buildRouter(cfg *config.Config, logger *zap.Logger, svc *service.Container) *gin.Engine {
	if !cfg.App.Debug {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.RequestLogger(logger))
	if !cfg.App.Debug && len(cfg.App.CORSOrigins) == 0 {
		logger.Warn("CORS: no origins configured in production — CORS headers will be omitted (same-origin enforced). Set app.cors_origins for cross-origin access.")
	}
	r.Use(middleware.CORS(cfg.App.CORSOrigins, cfg.App.Debug))

	handler.Register(r, cfg, logger, svc)

	if cfg.App.WebDir != "" {
		serveSPA(r, cfg.App.WebDir)
	}
	return r
}

// serveSPA serves the React build artifacts and falls back to index.html for
// non-API, non-asset paths so client-side routing keeps working.
func serveSPA(r *gin.Engine, webDir string) {
	assets := r.Group("/assets")
	assets.Use(func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Next()
	})
	assets.Static("/", filepath.Join(webDir, "assets"))
	brand := r.Group("/brand")
	brand.Use(func(c *gin.Context) {
		setNoCacheHeaders(c)
		c.Next()
	})
	brand.Static("/", filepath.Join(webDir, "brand"))
	for _, rootFile := range []string{"/favicon.ico", "/favicon.svg", "/artwork-cache-sw.js"} {
		filePath := filepath.Join(webDir, strings.TrimPrefix(rootFile, "/"))
		r.GET(rootFile, serveNoCacheFile(filePath))
		r.HEAD(rootFile, serveNoCacheFile(filePath))
	}
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if shouldBypassSPAFallback(path) {
			c.Status(http.StatusNotFound)
			return
		}
		serveSPAIndex(c, filepath.Join(webDir, "index.html"))
	})
}

func serveNoCacheFile(filePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		setNoCacheHeaders(c)
		if _, err := os.Stat(filePath); err != nil {
			c.Status(http.StatusNotFound)
			return
		}
		c.File(filePath)
	}
}

func serveSPAIndex(c *gin.Context, indexPath string) {
	setNoCacheHeaders(c)
	if _, err := os.Stat(indexPath); err != nil {
		c.String(http.StatusNotFound, "MediaStationGo web UI not found: %s", indexPath)
		return
	}
	c.File(indexPath)
}

func setNoCacheHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

func shouldBypassSPAFallback(path string) bool {
	if isFrontendLibraryRoute(path) {
		return false
	}
	lower := strings.ToLower(path)
	for _, exact := range []string{
		"/emby",
	} {
		if lower == exact {
			return true
		}
	}
	for _, prefix := range []string{
		"/api/",
		"/emby/",
		"/system/",
		"/users/",
		"/items/",
		"/shows/",
		"/library/",
		"/videos/",
		"/sessions/",
		"/displaypreferences/",
		"/branding/",
		"/localization/",
		"/startup/",
		"/quickconnect/",
		"/socket",
		"/embywebsocket",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func isFrontendLibraryRoute(path string) bool {
	const prefix = "/library/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	id := strings.TrimPrefix(path, prefix)
	if strings.Contains(id, "/") {
		return false
	}
	if len(id) != 36 {
		return false
	}
	for i, ch := range id {
		switch i {
		case 8, 13, 18, 23:
			if ch != '-' {
				return false
			}
		default:
			if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')) {
				return false
			}
		}
	}
	return true
}
