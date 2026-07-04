// Package database wires up GORM against the configured database and exposes
// startup migration helpers.
package database

import (
	"errors"
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// Open initialises the configured GORM database. database.type=auto chooses
// PostgreSQL when database.dsn is present and otherwise falls back to SQLite.
func Open(cfg *config.Config, log *zap.Logger) (*gorm.DB, error) {
	if cfg == nil {
		return nil, errors.New("database config is required")
	}
	dialect := normalizeDatabaseType(cfg.Database.Type)
	if dialect == "auto" {
		dialect = effectiveAutoDatabaseType(cfg)
	}
	dialector, err := databaseDialector(cfg, dialect)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger:                                   newGormLogger(log),
		PrepareStmt:                              true,
		DisableForeignKeyConstraintWhenMigrating: false,
	})
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}
	if dialect == "sqlite" {
		installSQLiteWriteGate(db)
	}
	if err := configureConnectionPool(db, cfg); err != nil {
		return nil, err
	}
	return db, nil
}

func newGormLogger(log *zap.Logger) logger.Interface {
	if log == nil {
		log = zap.NewNop()
	}
	return logger.New(
		zapStdLogger{log: log},
		logger.Config{
			SlowThreshold:             0,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)
}

func configureConnectionPool(db *gorm.DB, cfg *config.Config) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("gorm sqldb: %w", err)
	}
	if cfg.Database.MaxOpenConns > 0 {
		sqlDB.SetMaxOpenConns(cfg.Database.MaxOpenConns)
	}
	if cfg.Database.MaxIdleConns > 0 {
		sqlDB.SetMaxIdleConns(cfg.Database.MaxIdleConns)
	}
	return nil
}

func normalizeDatabaseType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return "auto"
	case "sqlite", "sqlite3":
		return "sqlite"
	case "postgres", "postgresql", "pg":
		return "postgres"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func effectiveAutoDatabaseType(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.Database.DSN) != "" {
		return "postgres"
	}
	return "sqlite"
}

func databaseDialector(cfg *config.Config, dialect string) (gorm.Dialector, error) {
	switch dialect {
	case "sqlite":
		return sqlite.Open(buildSQLiteDSN(cfg)), nil
	case "postgres":
		dsn := strings.TrimSpace(cfg.Database.DSN)
		if dsn == "" {
			return nil, fmt.Errorf("database.dsn is required when database.type=postgres")
		}
		return postgres.Open(dsn), nil
	default:
		return nil, fmt.Errorf("unsupported database.type %q (supported: sqlite, postgres)", cfg.Database.Type)
	}
}

// zapStdLogger adapts a *zap.Logger to GORM's tiny logger interface.
type zapStdLogger struct{ log *zap.Logger }

func (z zapStdLogger) Printf(format string, args ...interface{}) {
	if z.log == nil {
		return
	}
	z.log.Sugar().Infof(format, args...)
}
