package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func openSQLiteMigrationSource(cfg *config.Config, sqlitePath string) (*gorm.DB, error) {
	srcCfg := *cfg
	srcCfg.Database.Type = "sqlite"
	srcCfg.Database.DBPath = sqlitePath
	return gorm.Open(sqlite.Open(buildSQLiteDSN(&srcCfg)), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func sqliteMigrationSourcePath(cfg *config.Config, log *zap.Logger) (string, error) {
	configured := strings.TrimSpace(cfg.Database.DBPath)
	if configured != "" {
		exists, err := regularFileExists(configured)
		if err != nil {
			return "", fmt.Errorf("stat sqlite migration source: %w", err)
		}
		if exists {
			return configured, nil
		}
	}

	fallback := filepath.Join(strings.TrimSpace(cfg.App.DataDir), "mediastation.db")
	if fallback == "" || sameCleanPath(configured, fallback) {
		return "", nil
	}
	exists, err := regularFileExists(fallback)
	if err != nil {
		return "", fmt.Errorf("stat default sqlite migration source: %w", err)
	}
	if !exists {
		return "", nil
	}
	if log != nil && configured != "" {
		log.Warn("configured sqlite migration source not found; using data-dir default",
			zap.String("configured", configured),
			zap.String("fallback", fallback))
	}
	return fallback, nil
}

func regularFileExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return !info.IsDir(), nil
}

func sameCleanPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
