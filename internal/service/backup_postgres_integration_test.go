package service

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/ShukeBta/MediaStationGo/internal/config"
)

// Set MEDIASTATION_TEST_POSTGRES_DSN to a PostgreSQL URL with CREATE DATABASE
// permission and put pg_dump/pg_restore on PATH. Every test creates and drops
// its own randomly named database; the supplied database is never restored.
func TestPostgresBackupRestoreRoundTrip(t *testing.T) {
	for _, format := range []string{"url", "keyword"} {
		t.Run(format, func(t *testing.T) { testPostgresBackupRestoreRoundTrip(t, format) })
	}
}

func testPostgresBackupRestoreRoundTrip(t *testing.T, format string) {
	db, cfg := postgresBackupTestDatabase(t)
	u, _ := url.Parse(cfg.Database.DSN)
	if format == "keyword" {
		password, _ := u.User.Password()
		quote := func(s string) string { return "'" + strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s) + "'" }
		cfg.Database.DSN = "host=" + quote(u.Hostname()) + " port=" + quote(u.Port()) + " dbname=" + quote(strings.TrimPrefix(u.Path, "/")) + " user=" + quote(u.User.Username()) + " password=" + quote(password) + " sslmode=disable TimeZone=Asia/Hong_Kong"
	} else {
		query := u.Query()
		query.Set("TimeZone", "Asia/Hong_Kong")
		u.RawQuery = query.Encode()
		cfg.Database.DSN = u.String()
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for _, statement := range []string{
		`CREATE TABLE backup_parent (id bigserial PRIMARY KEY, name text NOT NULL, payload bytea, settings jsonb)`,
		`CREATE TABLE backup_child (id bigserial PRIMARY KEY, parent_id bigint REFERENCES backup_parent(id))`,
		`INSERT INTO backup_parent (name, payload, settings) VALUES ('保留影片', decode('00ff01', 'hex'), '{"quality":"4K"}'), ('Second', NULL, NULL)`,
		`INSERT INTO backup_child (parent_id) VALUES (1)`,
	} {
		if err := db.WithContext(ctx).Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	backup := NewBackupService(cfg, zap.NewNop(), db)
	info, err := backup.Create(ctx)
	if err != nil {
		t.Fatalf("create PostgreSQL backup: %v", err)
	}
	if !strings.HasSuffix(info.Filename, ".dump") || info.Size == 0 {
		t.Fatalf("unexpected PostgreSQL backup: %+v", info)
	}
	data, err := os.ReadFile(info.Path)
	if err != nil || !strings.HasPrefix(string(data), "PGDMP") {
		t.Fatalf("backup is not a native PostgreSQL archive: %v", err)
	}
	items, err := backup.List()
	if err != nil || len(items) != 1 || items[0].Filename != info.Filename {
		t.Fatalf("backup list = %+v, %v", items, err)
	}
	if err := db.Exec(`UPDATE backup_parent SET name = 'Changed' WHERE id = 1`).Error; err != nil {
		t.Fatal(err)
	}
	// Leave the table of contents intact but truncate the archive's data, so
	// restore fails after starting its transaction and dropping live objects.
	brokenName := "truncated.dump"
	if err := os.WriteFile(filepath.Join(backup.backupDir(), brokenName), data[:len(data)-32], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := backup.Restore(ctx, brokenName); err == nil {
		t.Fatal("truncated archive unexpectedly restored")
	}
	var name string
	if err := db.Raw(`SELECT name FROM backup_parent WHERE id = 1`).Scan(&name).Error; err != nil || name != "Changed" {
		t.Fatalf("failed restore changed the live database: name=%q, %v", name, err)
	}
	bot := &TelegramBotService{backup: backup}
	if reply := bot.cmdMgoRestoreDB(ctx, []string{info.Filename}); !strings.Contains(reply.Text, "需要确认") {
		t.Fatalf("Telegram restore did not require confirmation: %q", reply.Text)
	}
	if reply := bot.cmdMgoRestoreDB(ctx, []string{info.Filename, "confirm"}); !strings.Contains(reply.Text, "PostgreSQL 数据库已从备份恢复") {
		t.Fatalf("Telegram PostgreSQL restore failed: %q", reply.Text)
	}
	if err := db.Raw(`SELECT name FROM backup_parent WHERE id = 1`).Scan(&name).Error; err != nil || name != "保留影片" {
		t.Fatalf("restored name = %q, %v", name, err)
	}
	var encoded string
	if err := db.Raw(`SELECT encode(payload, 'hex') || ':' || (settings->>'quality') FROM backup_parent WHERE id = 1`).Scan(&encoded).Error; err != nil || encoded != "00ff01:4K" {
		t.Fatalf("restored binary/JSON data = %q, %v", encoded, err)
	}
	var id int64
	if err := db.Raw(`INSERT INTO backup_parent (name) VALUES ('After restore') RETURNING id`).Scan(&id).Error; err != nil || id != 3 {
		t.Fatalf("restored sequence = %d, %v", id, err)
	}
	if err := db.Exec(`INSERT INTO backup_child (parent_id) VALUES (999999)`).Error; err == nil {
		t.Fatal("restore lost the foreign key constraint")
	}
	if reply := bot.cmdMgoBackupDB(ctx); !strings.Contains(reply.Text, ".dump") || !strings.Contains(reply.Text, "数据库备份完成") {
		t.Fatalf("Telegram backup failed: %q", reply.Text)
	}
	if reply := bot.cmdMgoRestoreDB(ctx, []string{"list"}); !strings.Contains(reply.Text, info.Filename) {
		t.Fatalf("Telegram backup list omitted archive: %q", reply.Text)
	}
	if err := backup.Delete(info.Filename); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Fatalf("backup was not deleted: %v", err)
	}
}

func postgresBackupTestDatabase(t *testing.T) (*gorm.DB, *config.Config) {
	t.Helper()
	dsn := os.Getenv("MEDIASTATION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set MEDIASTATION_TEST_POSTGRES_DSN to run PostgreSQL backup integration tests")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil {
		t.Fatal("MEDIASTATION_TEST_POSTGRES_DSN must be a PostgreSQL URL")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open test PostgreSQL server: %v", err)
	}
	name := "msgo_backup_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if err := admin.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, name)).Error; err != nil {
		t.Fatal(err)
	}
	var db *gorm.DB
	t.Cleanup(func() {
		if db != nil {
			if sqlDB, err := db.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		if err := admin.Exec(fmt.Sprintf(`DROP DATABASE "%s" WITH (FORCE)`, name)).Error; err != nil {
			t.Errorf("drop disposable PostgreSQL database: %v", err)
		}
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	u.Path = "/" + name
	cfg := &config.Config{}
	cfg.App.DataDir = t.TempDir()
	cfg.Database.Type = "auto"
	cfg.Database.DSN = u.String()
	db, err = gorm.Open(postgres.Open(cfg.Database.DSN), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return db, cfg
}
