package database

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

const sqliteRestorePendingSuffix = ".restore-pending"

// StageSQLiteRestore queues a checked snapshot beside the database. The live
// database and its WAL remain untouched until all connections close on restart.
func StageSQLiteRestore(ctx context.Context, cfg *config.Config, snapshot string) error {
	dbPath, err := sqliteRestoreDatabasePath(cfg)
	if err != nil {
		return err
	}
	pending := dbPath + sqliteRestorePendingSuffix
	if _, err := os.Lstat(pending); err == nil {
		return errors.New("a SQLite restore is already pending; restart the server to apply it")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	in, err := os.Open(snapshot) // #nosec G304 -- caller supplies a validated backup file.
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.CreateTemp(filepath.Dir(dbPath), ".sqlite-restore-*")
	if err != nil {
		return err
	}
	tmp := out.Name()
	defer os.Remove(tmp)
	_, copyErr := io.Copy(out, &sqliteRestoreReader{ctx: ctx, reader: in})
	syncErr := out.Sync()
	closeErr := out.Close()
	if err := errors.Join(copyErr, syncErr, closeErr); err != nil {
		return err
	}
	if err := validateSQLiteSnapshot(ctx, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, pending)
}

// applyPendingSQLiteRestore runs before opening any SQLite connection. Preserve
// the old database and its journals together so the previous state remains usable.
func applyPendingSQLiteRestore(cfg *config.Config, log *zap.Logger) error {
	dbPath, err := sqliteRestoreDatabasePath(cfg)
	if err != nil {
		// In-memory databases never have a persisted restore request.
		return nil
	}
	pending := dbPath + sqliteRestorePendingSuffix
	if _, err := os.Lstat(pending); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := validateSQLiteSnapshot(context.Background(), pending); err != nil {
		return fmt.Errorf("pending SQLite restore is invalid: %w", err)
	}
	previous := dbPath + ".before-restore-" + time.Now().Format("20060102_150405.000000000")
	rollback, err := preserveSQLiteDatabase(dbPath, previous)
	if err != nil {
		return err
	}
	if err := os.Rename(pending, dbPath); err != nil {
		return errors.Join(err, rollback())
	}
	if log != nil {
		log.Warn("SQLite backup restored before opening database", zap.String("previous_database", previous))
	}
	return nil
}

func preserveSQLiteDatabase(dbPath, previous string) (func() error, error) {
	var moved []string
	rollback := func() error {
		var result error
		for i := len(moved) - 1; i >= 0; i-- {
			result = errors.Join(result, os.Rename(previous+moved[i], dbPath+moved[i]))
		}
		return result
	}
	for _, suffix := range []string{"", "-wal", "-shm", "-journal"} {
		info, err := os.Lstat(dbPath + suffix)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, errors.Join(err, rollback())
		}
		if !info.Mode().IsRegular() {
			return nil, errors.Join(fmt.Errorf("SQLite restore path is not a regular file: %s", dbPath+suffix), rollback())
		}
		if err := os.Rename(dbPath+suffix, previous+suffix); err != nil {
			return nil, errors.Join(err, rollback())
		}
		moved = append(moved, suffix)
	}
	return rollback, nil
}

func sqliteRestoreDatabasePath(cfg *config.Config) (string, error) {
	if cfg == nil || strings.TrimSpace(cfg.Database.DBPath) == "" || strings.Contains(cfg.Database.DBPath, ":memory:") {
		return "", errors.New("SQLite restore requires a persistent database path")
	}
	return filepath.Abs(cfg.Database.DBPath)
}

func validateSQLiteSnapshot(ctx context.Context, path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() < 100 {
		return errors.New("invalid SQLite snapshot")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	uriPath := filepath.ToSlash(absolute)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=ro"}
	db, err := gorm.Open(sqlite.Open(u.String()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open SQLite snapshot: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	var checks []string
	if err := db.WithContext(ctx).Raw("PRAGMA quick_check").Scan(&checks).Error; err != nil {
		return fmt.Errorf("check SQLite snapshot: %w", err)
	}
	if len(checks) != 1 || checks[0] != "ok" {
		return errors.New("SQLite snapshot failed integrity check")
	}
	return nil
}

type sqliteRestoreReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *sqliteRestoreReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
