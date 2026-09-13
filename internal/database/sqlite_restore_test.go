package database

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

func TestSQLiteStartupRejectsCorruptPendingRestoreWithoutChangingLiveFile(t *testing.T) {
	cfg := sqliteRestoreTestDatabase(t)
	before, err := os.ReadFile(cfg.Database.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	pending := cfg.Database.DBPath + sqliteRestorePendingSuffix
	if err := os.WriteFile(pending, bytes.Repeat([]byte("corrupt"), 100), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(cfg, zap.NewNop()); err == nil {
		t.Fatal("startup accepted a corrupt pending snapshot")
	}
	after, err := os.ReadFile(cfg.Database.DBPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("startup changed the original database: %v", err)
	}
}

func TestSQLiteStartupRollsBackWhenPreservingWALFails(t *testing.T) {
	cfg := sqliteRestoreTestDatabase(t)
	before, err := os.ReadFile(cfg.Database.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := StageSQLiteRestore(t.Context(), cfg, cfg.Database.DBPath); err != nil {
		t.Fatal(err)
	}
	// A non-file WAL path fails after the main DB has already been moved.
	if err := os.Mkdir(cfg.Database.DBPath+"-wal", 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(cfg, zap.NewNop()); err == nil {
		t.Fatal("startup ignored an invalid WAL path")
	}
	after, err := os.ReadFile(cfg.Database.DBPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("failed restore did not roll back the original database: %v", err)
	}
	if _, err := os.Stat(cfg.Database.DBPath + sqliteRestorePendingSuffix); err != nil {
		t.Fatalf("failed restore lost the staged snapshot: %v", err)
	}
}

func TestSQLiteStartupPreservesRollbackJournalWithPreviousDatabase(t *testing.T) {
	cfg := sqliteRestoreTestDatabase(t)
	db, err := Open(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, statement := range []string{
		`PRAGMA journal_mode=PERSIST`,
		`INSERT INTO original_data VALUES ('snapshot')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := StageSQLiteRestore(t.Context(), cfg, cfg.Database.DBPath); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE original_data SET value = 'live'`).Error; err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	journal, err := os.ReadFile(cfg.Database.DBPath + "-journal")
	if err != nil || len(journal) == 0 {
		t.Fatalf("rollback journal was not created: %v", err)
	}
	// Journal recovery must never apply the previous database's pages to the
	// restored snapshot. Keep the sidecar together with the saved database.
	if err := applyPendingSQLiteRestore(cfg, zap.NewNop()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.Database.DBPath + "-journal"); !os.IsNotExist(err) {
		t.Fatalf("previous rollback journal remains beside the restored snapshot: %v", err)
	}
	previousJournals, err := filepath.Glob(cfg.Database.DBPath + ".before-restore-*-journal")
	if err != nil || len(previousJournals) != 1 {
		t.Fatalf("previous rollback journal was not retained: %v", err)
	}
	retained, err := os.ReadFile(previousJournals[0])
	if err != nil || !bytes.Equal(retained, journal) {
		t.Fatalf("saved rollback journal changed: %v", err)
	}
	restored, err := Open(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	restoredSQL, _ := restored.DB()
	t.Cleanup(func() { _ = restoredSQL.Close() })
	var value string
	if err := restored.Raw(`SELECT value FROM original_data`).Scan(&value).Error; err != nil || value != "snapshot" {
		t.Fatalf("restored value = %q, %v", value, err)
	}
}

func sqliteRestoreTestDatabase(t *testing.T) *config.Config {
	t.Helper()
	cfg := &config.Config{}
	cfg.Database.Type = "sqlite"
	cfg.Database.DBPath = filepath.Join(t.TempDir(), "live.db")
	db, err := Open(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	if err := db.Exec(`CREATE TABLE original_data (value text)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	return cfg
}
