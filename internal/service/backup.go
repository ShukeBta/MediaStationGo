// Package service manages native SQLite and PostgreSQL backups under data/backups.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

type BackupService struct {
	cfg *config.Config
	log *zap.Logger
	db  *gorm.DB
	mu  sync.Mutex

	runCommand backupCommandRunner
}

func NewBackupService(cfg *config.Config, log *zap.Logger, db *gorm.DB) *BackupService {
	if log == nil {
		log = zap.NewNop()
	}
	return &BackupService{cfg: cfg, log: log, db: db, runCommand: runBackupCommand}
}

type BackupInfo struct {
	Filename     string    `json:"filename"`
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"created_at"`
	DatabaseType string    `json:"database_type"`
}

func (b *BackupService) backupDir() string {
	return filepath.Join(b.cfg.App.DataDir, "backups")
}

func (b *BackupService) backupFilePath(filename string) (string, error) {
	if !isValidBackupFilename(filename) {
		return "", errors.New("invalid filename")
	}
	dir, err := filepath.Abs(b.backupDir())
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filename), nil
}

func (b *BackupService) databaseType() (string, error) {
	if b == nil || b.cfg == nil || b.db == nil || b.db.Dialector == nil {
		return "", errors.New("backup database is unavailable")
	}
	switch name := b.db.Dialector.Name(); name {
	case "sqlite", "postgres":
		return name, nil
	default:
		return "", fmt.Errorf("backup is not supported for database %q", name)
	}
}

// RestoreAppliesOnRestart distinguishes SQLite's queued file swap from a
// PostgreSQL restore that has already committed its database transaction.
func (b *BackupService) RestoreAppliesOnRestart() bool {
	dialect, _ := b.databaseType()
	return dialect == "sqlite"
}

// Create publishes a backup only after the database snapshot has completed.
// Temporary files are excluded from List and removed after any failure.
func (b *BackupService) Create(ctx context.Context) (*BackupInfo, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	dialect, err := b.databaseType()
	if err != nil {
		return nil, err
	}
	tmp, err := createBackupTempFile(b.backupDir())
	if err != nil {
		return nil, err
	}
	defer os.Remove(tmp)
	ext := ".db"
	if dialect == "postgres" {
		ext = ".dump"
		err = b.createPostgres(ctx, tmp)
	} else {
		err = b.db.WithContext(ctx).Exec("VACUUM INTO ?", tmp).Error
	}
	if err != nil {
		return nil, fmt.Errorf("create %s backup: %w", dialect, err)
	}
	if err := validateBackupHeader(tmp, dialect); err != nil {
		return nil, err
	}
	dst := strings.TrimSuffix(tmp, ".partial") + ext
	if err := os.Rename(tmp, dst); err != nil {
		return nil, err
	}
	info, err := backupFileInfo(dst)
	if err != nil {
		return nil, err
	}
	b.log.Info("backup created", zap.String("database", dialect), zap.String("path", dst), zap.Int64("size", info.Size))
	return info, nil
}

func (b *BackupService) List() ([]BackupInfo, error) {
	entries, err := os.ReadDir(b.backupDir())
	if errors.Is(err, os.ErrNotExist) {
		return []BackupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]BackupInfo, 0, len(entries))
	for _, e := range entries {
		if !e.Type().IsRegular() || !isValidBackupFilename(e.Name()) {
			continue
		}
		if info, err := backupFileInfo(filepath.Join(b.backupDir(), e.Name())); err == nil {
			out = append(out, *info)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (b *BackupService) Delete(filename string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	path, err := b.backupFilePath(filename)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// Restore rejects cross-database formats before touching live data. PostgreSQL
// restores all archived objects in one transaction; callers restart application
// services afterward to discard cached settings and prepared statements.
func (b *BackupService) Restore(ctx context.Context, filename string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	dialect, err := b.databaseType()
	if err != nil {
		return err
	}
	src, err := b.backupFilePath(filename)
	if err != nil {
		return err
	}
	if backupFilenameDatabaseType(filename) != dialect {
		return fmt.Errorf("backup format does not match the active %s database", dialect)
	}
	if err := validateBackupHeader(src, dialect); err != nil {
		return err
	}
	if dialect == "postgres" {
		err = b.restorePostgres(ctx, src)
	} else {
		err = b.restoreSQLite(ctx, src)
	}
	if err != nil {
		return err
	}
	b.log.Warn("database restore prepared — restart the server", zap.String("database", dialect), zap.String("backup", filename))
	return nil
}

func backupFilenameDatabaseType(name string) string {
	switch filepath.Ext(name) {
	case ".db":
		return "sqlite"
	case ".dump":
		return "postgres"
	default:
		return ""
	}
}

func isValidBackupFilename(name string) bool {
	return name != "" && strings.TrimSpace(name) == name &&
		!strings.ContainsAny(name, "/\\:\x00\r\n") && !strings.Contains(name, "..") &&
		backupFilenameDatabaseType(name) != ""
}
