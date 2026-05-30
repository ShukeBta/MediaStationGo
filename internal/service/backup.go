// Package service — database hot backup and restore.
//
// BackupService uses SQLite's "VACUUM INTO" (available since 3.27.0) to
// produce a consistent copy of the database without blocking writers.
// Backup files are stored under {data_dir}/backups/ with a timestamp
// suffix. The admin UI can list / download / delete / restore them.
package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// BackupService manages database backups.
type BackupService struct {
	cfg *config.Config
	log *zap.Logger
	db  *gorm.DB
}

// NewBackupService is the constructor.
func NewBackupService(cfg *config.Config, log *zap.Logger, db *gorm.DB) *BackupService {
	return &BackupService{cfg: cfg, log: log, db: db}
}

// BackupInfo describes one stored backup file.
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func (b *BackupService) backupDir() string {
	return filepath.Join(b.cfg.App.DataDir, "backups")
}

// Create produces a new hot backup via "VACUUM INTO".
func (b *BackupService) Create(ctx context.Context) (*BackupInfo, error) {
	dir := b.backupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	ts := time.Now().Format("20060102_150405")
	dst := filepath.Join(dir, fmt.Sprintf("mediastation_%s.db", ts))

	// VACUUM INTO creates a clean, non-WAL copy atomically.
	if err := b.db.WithContext(ctx).Exec("VACUUM INTO ?", dst).Error; err != nil {
		return nil, fmt.Errorf("VACUUM INTO failed: %w", err)
	}
	stat, err := os.Stat(dst)
	if err != nil {
		return nil, err
	}
	b.log.Info("backup created", zap.String("path", dst), zap.Int64("size", stat.Size()))
	return &BackupInfo{
		Filename:  filepath.Base(dst),
		Path:      dst,
		Size:      stat.Size(),
		CreatedAt: stat.ModTime(),
	}, nil
}

// List returns every backup file in the backup directory, newest first.
func (b *BackupService) List() ([]BackupInfo, error) {
	dir := b.backupDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]BackupInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, BackupInfo{
			Filename:  e.Name(),
			Path:      filepath.Join(dir, e.Name()),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

// Delete removes a single backup file.
func (b *BackupService) Delete(filename string) error {
	if !isValidBackupFilename(filename) {
		return errors.New("invalid filename")
	}
	path := filepath.Join(b.backupDir(), filename)
	return os.Remove(path)
}

// Restore copies a backup over the live database using VACUUM INTO in
// reverse. WARNING: this is destructive — the live DB will be replaced.
// Callers should shut down the server after this call.
func (b *BackupService) Restore(ctx context.Context, filename string) error {
	if !isValidBackupFilename(filename) {
		return errors.New("invalid filename")
	}
	src := filepath.Join(b.backupDir(), filename)
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("backup not found: %s", filename)
	}
	dbPath := b.cfg.Database.DBPath
	// Strategy: rename live → live.old, copy backup → live, delete old.
	old := dbPath + ".before_restore"
	if err := os.Rename(dbPath, old); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	data, err := os.ReadFile(src)
	if err != nil {
		// Revert
		_ = os.Rename(old, dbPath)
		return err
	}
	if err := os.WriteFile(dbPath, data, 0o644); err != nil {
		_ = os.Rename(old, dbPath)
		return err
	}
	_ = os.Remove(old)
	// Also remove WAL/SHM so SQLite opens the fresh file cleanly.
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	b.log.Warn("database restored from backup — restart the server",
		zap.String("backup", filename))
	return nil
}

// isValidBackupFilename rejects path traversal attempts and non-.db files.
func isValidBackupFilename(name string) bool {
	if name == "" {
		return false
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return false
	}
	if !strings.HasSuffix(name, ".db") {
		return false
	}
	return true
}
