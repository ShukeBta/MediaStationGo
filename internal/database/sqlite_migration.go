package database

import (
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// MigrateSQLiteToCurrentIfNeeded copies an existing SQLite database into
// PostgreSQL. Redis is not migrated because it is a rebuildable cache, not a
// source of truth.
const sqliteMigrationCompleteSettingKey = "database.sqlite_migration_complete"

func MigrateSQLiteToCurrentIfNeeded(cfg *config.Config, target *gorm.DB, log *zap.Logger) error {
	if cfg == nil || target == nil || target.Dialector == nil || target.Dialector.Name() != "postgres" {
		return nil
	}
	sqlitePath, err := sqliteMigrationSourcePath(cfg, log)
	if err != nil {
		return err
	}
	if sqlitePath == "" {
		return nil
	}
	if complete, err := sqliteMigrationMarkedComplete(target); err != nil {
		return err
	} else if complete {
		if log != nil {
			log.Info("skip sqlite to postgres migration: already completed")
		}
		return nil
	}

	src, err := openSQLiteMigrationSource(cfg, sqlitePath)
	if err != nil {
		return fmt.Errorf("open sqlite migration source: %w", err)
	}
	sqlDB, err := src.DB()
	if err == nil {
		defer sqlDB.Close()
	}

	started := time.Now()
	if err := resetBootstrapTargetBeforeSQLiteMigrationIfSafe(src, target, log); err != nil {
		return err
	}
	copied, err := copyModelTables(src, target, 500)
	if err != nil {
		return err
	}
	if err := markSQLiteMigrationComplete(target); err != nil {
		return err
	}
	if log != nil {
		log.Info("sqlite data migrated to postgres",
			zap.String("source", sqlitePath),
			zap.Int64("rows", copied),
			zap.Duration("duration", time.Since(started)))
	}
	return nil
}
