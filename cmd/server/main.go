// Package main is the MediaStationGo HTTP server entry point.
//
// MediaStationGo is a Go rewrite of the original Python MediaStation project,
// adopting the same tech stack as cropflre/nowen-video:
//
//	Backend:  Go 1.25 + Gin + GORM + SQLite (WAL) + Viper + Zap + JWT
//	Frontend: React 18 + Vite + Tailwind + Zustand + HLS.js
//
// The binary embeds the SPA build artifacts at /app/web/dist and serves them
// alongside the JSON REST API at /api/* and the WebSocket hub at /api/ws.
package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/handler"
	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// version is overwritten at build time via -ldflags="-X main.version=...".
var version = "dev"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config load failed: %v\n", err)
		os.Exit(1)
	}

	logger, err := newLogger(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger init failed: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = logger.Sync() }()

	logger.Info("starting MediaStationGo",
		zap.String("version", version),
		zap.Int("port", cfg.App.Port),
		zap.String("data_dir", cfg.App.DataDir),
	)

	// Ensure data / cache / web dirs exist.
	for _, d := range []string{cfg.App.DataDir, cfg.Cache.CacheDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			logger.Fatal("create dir failed", zap.String("dir", d), zap.Error(err))
		}
	}

	db, err := database.Open(cfg, logger)
	if err != nil {
		logger.Fatal("database open failed", zap.Error(err))
	}
	if err := database.AutoMigrate(db); err != nil {
		logger.Fatal("auto-migrate failed", zap.Error(err))
	}

	repos := repository.New(db)
	services := service.New(cfg, logger, repos)

	if err := services.Auth.SeedAdmin(context.Background()); err != nil {
		logger.Warn("seed admin failed", zap.Error(err))
	}
	services.Boot()

	router := buildRouter(cfg, logger, services)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
	}

	go func() {
		localIP := getLocalIP()
		publicIP := getPublicIP(3 * time.Second)
		if publicIP != "" {
			logger.Info("server is ready",
				zap.String("local", fmt.Sprintf("http://%s:%d", localIP, cfg.App.Port)),
				zap.String("public", fmt.Sprintf("http://%s:%d", publicIP, cfg.App.Port)),
				zap.String("listen", srv.Addr),
			)
		} else {
			logger.Info("server is ready",
				zap.String("local", fmt.Sprintf("http://%s:%d", localIP, cfg.App.Port)),
				zap.String("listen", srv.Addr),
			)
		}
		// 自动启动 Telegram 长轮询（无需公网 Webhook）
		services.TelegramBot.StartPolling(context.Background())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("listen failed", zap.Error(err))
		}
	}()

	// Graceful shutdown.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	logger.Info("shutdown requested")

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
	services.Close()
	logger.Info("MediaStationGo stopped")
}

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

	// Static SPA fallback.
	if cfg.App.WebDir != "" {
		serveSPA(r, cfg.App.WebDir)
	}
	return r
}

// serveSPA serves the React build artifacts and falls back to index.html for
// non-API, non-asset paths so client-side routing keeps working.
func serveSPA(r *gin.Engine, webDir string) {
	r.Static("/assets", filepath.Join(webDir, "assets"))
	r.StaticFile("/favicon.ico", filepath.Join(webDir, "favicon.ico"))
	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		// Do not swallow API / Emby compatibility routes; clients expect JSON
		// or 404, not the React index.html fallback.
		if shouldBypassSPAFallback(path) {
			c.Status(http.StatusNotFound)
			return
		}
		c.File(filepath.Join(webDir, "index.html"))
	})
}

func shouldBypassSPAFallback(path string) bool {
	lower := strings.ToLower(path)
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
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func newLogger(cfg *config.Config) (*zap.Logger, error) {
	if cfg.App.Debug {
		return zap.NewDevelopment()
	}
	return zap.NewProduction()
}

// getLocalIP returns the first non-loopback IPv4 address of the machine.
// Falls back to "localhost" if no suitable interface is found.
func getLocalIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "localhost"
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if ip := v.IP.To4(); ip != nil {
					return ip.String()
				}
			}
		}
	}
	return "localhost"
}

// getPublicIP tries to detect the public-facing IP by querying ipify.org.
// Returns empty string if detection fails (e.g. no internet, timeout).
func getPublicIP(timeout time.Duration) string {
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, err := resp.Body.Read(buf)
	if err != nil || n == 0 || resp.StatusCode != http.StatusOK {
		return ""
	}
	return string(buf[:n])
}
