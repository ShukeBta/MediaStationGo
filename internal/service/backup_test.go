package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
)

func TestSQLiteBackupRestoreAppliesOnlyAfterRestart(t *testing.T) {
	cfg := &config.Config{}
	cfg.App.DataDir = filepath.Join(t.TempDir(), "数据库 #100%")
	if err := os.MkdirAll(cfg.App.DataDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg.Database.Type = "sqlite"
	cfg.Database.DBPath = filepath.Join(cfg.App.DataDir, "live.db")
	cfg.Database.WALMode = true
	cfg.Database.BusyTimeout = 1000
	db, err := database.Open(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec(`CREATE TABLE restore_marker (id integer PRIMARY KEY, value text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO restore_marker VALUES (1, 'original')`).Error; err != nil {
		t.Fatal(err)
	}
	backup := NewBackupService(cfg, zap.NewNop(), db)
	info, err := backup.Create(t.Context())
	if err != nil || info.DatabaseType != "sqlite" || !strings.HasSuffix(info.Filename, ".db") {
		t.Fatalf("SQLite backup = %+v, %v", info, err)
	}
	second, err := backup.Create(t.Context())
	if err != nil || second.Filename == info.Filename {
		t.Fatalf("consecutive backups must have different names: %+v, %v", second, err)
	}
	if err := db.Exec(`UPDATE restore_marker SET value = 'live'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := backup.Restore(t.Context(), info.Filename); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.Raw(`SELECT value FROM restore_marker`).Scan(&value).Error; err != nil || value != "live" {
		t.Fatalf("staging restore changed the open database: %q, %v", value, err)
	}
	if err := backup.Restore(t.Context(), second.Filename); err == nil {
		t.Fatal("a second restore must not silently replace a pending restore")
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := database.Open(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	reopenedSQL, _ := reopened.DB()
	t.Cleanup(func() { _ = reopenedSQL.Close() })
	if err := reopened.Raw(`SELECT value FROM restore_marker`).Scan(&value).Error; err != nil || value != "original" {
		t.Fatalf("restored database = %q, %v", value, err)
	}
	previous, err := filepath.Glob(cfg.Database.DBPath + ".before-restore-*")
	if err != nil || len(previous) == 0 {
		t.Fatalf("pre-restore database was not retained: %v", err)
	}
}

func TestBackupRejectsWrongFormatsAndCorruptSQLiteBeforeRestoring(t *testing.T) {
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	cfg.Database.DBPath = filepath.Join(cfg.App.DataDir, "live.db")
	db, err := database.Open(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec(`CREATE TABLE retained (value text)`).Error; err != nil {
		t.Fatal(err)
	}
	backup := NewBackupService(cfg, zap.NewNop(), db)
	if err := os.MkdirAll(backup.backupDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string]string{
		"wrong.dump": "PGDMP" + strings.Repeat("x", 100),
		"bad.db":     "SQLite format 3\x00" + strings.Repeat("x", 500),
		"text.db":    "not a database",
	} {
		writeTemp(t, backup.backupDir(), name, content)
		if err := backup.Restore(t.Context(), name); err == nil {
			t.Fatalf("accepted invalid backup %q", name)
		}
	}
	if _, err := os.Stat(cfg.Database.DBPath + ".restore-pending"); !os.IsNotExist(err) {
		t.Fatalf("invalid backup was staged for restart: %v", err)
	}
	if err := db.Exec(`INSERT INTO retained VALUES ('still writable')`).Error; err != nil {
		t.Fatalf("live database was damaged: %v", err)
	}
}

func TestPostgresBackupFailureCleansPartialFilesAndRedactsPassword(t *testing.T) {
	backup := fakePostgresBackup(t)
	backup.runCommand = func(_ context.Context, _ string, args, env []string) (string, error) {
		if strings.Contains(strings.Join(args, " "), "secret pass") {
			t.Fatal("database password appeared in process arguments")
		}
		if !containsString(env, "PGPASSWORD=secret pass") {
			t.Fatal("database password was not passed through the client environment")
		}
		for _, arg := range args {
			if strings.HasPrefix(arg, "--file=") {
				if err := os.WriteFile(strings.TrimPrefix(arg, "--file="), []byte("partial"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}
		return "authentication failed for secret pass", errors.New("exit status 1")
	}
	if _, err := backup.Create(t.Context()); err == nil || strings.Contains(err.Error(), "secret pass") {
		t.Fatalf("backup failure leaked credentials or was ignored: %v", err)
	}
	entries, err := os.ReadDir(backup.backupDir())
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed backup left partial files: %+v, %v", entries, err)
	}
}

func TestBackupListIncludesBothFormatsAndSkipsPartialFiles(t *testing.T) {
	backup := fakePostgresBackup(t)
	if err := os.MkdirAll(backup.backupDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"old.db", "new.dump", "unfinished.partial", "notes.txt"} {
		writeTemp(t, backup.backupDir(), name, "data")
	}
	items, err := backup.List()
	if err != nil || len(items) != 2 {
		t.Fatalf("listed backups = %+v, %v", items, err)
	}
	kinds := map[string]bool{}
	for _, item := range items {
		kinds[item.DatabaseType] = true
	}
	if !kinds["sqlite"] || !kinds["postgres"] {
		t.Fatalf("backup types = %+v", kinds)
	}
}

func fakePostgresBackup(t *testing.T) *BackupService {
	t.Helper()
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	cfg.Database.Type = "auto"
	cfg.Database.DSN = "postgres://backup:secret%20pass@localhost/test?sslmode=disable"
	db := &gorm.DB{Config: &gorm.Config{Dialector: postgres.Open(cfg.Database.DSN)}}
	return NewBackupService(cfg, zap.NewNop(), db)
}

func TestPostgresBackupMissingClientProducesActionableError(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	backup := fakePostgresBackup(t)
	if _, err := backup.Create(t.Context()); err == nil || !strings.Contains(err.Error(), "install PostgreSQL client tools") {
		t.Fatalf("missing pg_dump error = %v", err)
	}
	items, err := backup.List()
	if err != nil || len(items) != 0 {
		t.Fatalf("missing client left a published backup: %+v, %v", items, err)
	}
}

func TestPostgresRestoreRejectsSQLiteWithoutTouchingMigrationSource(t *testing.T) {
	backup := fakePostgresBackup(t)
	backup.cfg.Database.DBPath = writeTemp(t, backup.cfg.App.DataDir, "legacy.db", "legacy migration source")
	if err := os.MkdirAll(backup.backupDir(), 0o750); err != nil {
		t.Fatal(err)
	}
	writeTemp(t, backup.backupDir(), "sqlite.db", "SQLite format 3\x00")
	if err := backup.Restore(t.Context(), "sqlite.db"); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("PostgreSQL accepted SQLite restore: %v", err)
	}
	if data, err := os.ReadFile(backup.cfg.Database.DBPath); err != nil || string(data) != "legacy migration source" {
		t.Fatalf("restore touched legacy SQLite source: %q, %v", data, err)
	}
}
