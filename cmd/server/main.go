// Package main is the MediaStationGo HTTP server entry point.
//
// MediaStationGo is a Go rewrite of the legacy Python implementation,
// adopting the same tech stack as cropflre/nowen-video:
//
//	Backend:  Go 1.25 + Gin + GORM + PostgreSQL/SQLite + Viper + Zap + JWT
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
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/handler"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// version is overwritten at build time via -ldflags="-X main.version=...".
var version = "dev"

func effectiveVersion(buildVersion string) string {
	buildVersion = strings.TrimSpace(buildVersion)
	if buildVersion != "" && buildVersion != "dev" {
		return buildVersion
	}
	if envVersion := strings.TrimSpace(os.Getenv("MEDIASTATION_VERSION")); envVersion != "" {
		return envVersion
	}
	if buildVersion == "" {
		return "dev"
	}
	return buildVersion
}

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

	appVersion := effectiveVersion(version)
	logger.Info("starting MediaStationGo",
		zap.String("version", appVersion),
		zap.Int("port", cfg.App.Port),
		zap.String("data_dir", cfg.App.DataDir),
	)

	// Ensure data / cache / web dirs exist.
	for _, d := range []string{cfg.App.DataDir, cfg.Cache.CacheDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			logger.Fatal("create dir failed", zap.String("dir", d), zap.Error(err))
		}
	}

	db, err := database.Open(cfg, logger)
	if err != nil {
		logger.Fatal("database open failed", zap.Error(err))
	}
	if err := waitForDatabase(db, logger); err != nil {
		logger.Fatal("database not ready", zap.Error(err))
	}
	if err := database.AutoMigrate(db); err != nil {
		logger.Fatal("auto-migrate failed", zap.Error(err))
	}
	if err := database.MigrateSQLiteToCurrentIfNeeded(cfg, db, logger); err != nil {
		logger.Fatal("sqlite to postgres migration failed", zap.Error(err))
	}

	repos := repository.New(db)
	service.ApplyRuntimeSettings(context.Background(), cfg, repos, logger)
	applyCPUThreadLimit(cfg, logger)
	services := service.NewWithVersion(cfg, logger, repos, appVersion)

	if repaired, err := services.RepairCloudPathMetadata(context.Background()); err != nil {
		logger.Warn("cloud path metadata repair failed", zap.Error(err))
	} else if repaired > 0 {
		logger.Info("cloud path metadata repair completed", zap.Int("media_count", repaired))
	}

	// 一次性清洗历史脏数据: 老版本把单集 episode id / 单集名写进整剧字段, 导致
	// 同一部剧被拆成多张单集卡。清空被污染的字段并重置为 pending(借后续重刮修正)。
	if cleaned, err := services.NormalizePollutedEpisodeMetadata(context.Background()); err != nil {
		logger.Warn("polluted episode metadata cleanup failed", zap.Error(err))
	} else if cleaned > 0 {
		logger.Info("polluted episode metadata cleanup completed", zap.Int("media_count", cleaned))
	}

	if err := services.Auth.SeedAdmin(context.Background()); err != nil {
		logger.Warn("seed admin failed", zap.Error(err))
	}

	router := buildRouter(cfg, logger, services)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.App.Port),
		Handler:           router,
		ReadHeaderTimeout: 15 * time.Second,
	}

	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		logger.Fatal("listen failed", zap.String("addr", srv.Addr), zap.Error(err))
	}
	localIP := getLocalIP()
	logger.Info("server is ready",
		zap.String("local", fmt.Sprintf("http://%s:%d", localIP, cfg.App.Port)),
		zap.String("listen", srv.Addr),
	)
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("listen failed", zap.Error(err))
		}
	}()
	go func() {
		if publicIP := getPublicIP(3 * time.Second); publicIP != "" {
			logger.Info("server public endpoint",
				zap.String("public", fmt.Sprintf("http://%s:%d", publicIP, cfg.App.Port)),
			)
		}
	}()
	go services.Boot()
	go handler.RunLicenseHeartbeatLoop(services.Context(), services)
	go services.TelegramBot.StartPolling(context.Background())

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
