package database

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOpenRequiresConfig(t *testing.T) {
	db, err := Open(nil, nil)
	if err == nil {
		t.Fatal("expected nil config to return an error")
	}
	if db != nil {
		t.Fatal("db should be nil when config is missing")
	}
	if !strings.Contains(err.Error(), "database config") {
		t.Fatalf("error = %v, want database config message", err)
	}
}

func TestOpenSQLiteWithNilLoggerConfiguresPool(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.Type = "sqlite"
	cfg.Database.DBPath = filepath.Join(t.TempDir(), "mediastation.db")
	cfg.Database.WALMode = true
	cfg.Database.BusyTimeout = 5000
	cfg.Database.CacheSize = -2000
	cfg.Database.MaxOpenConns = 3
	cfg.Database.MaxIdleConns = 2

	db, err := Open(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.Exec("SELECT 1").Error; err != nil {
		t.Fatal(err)
	}
	stats := sqlDB.Stats()
	if stats.MaxOpenConnections != 3 {
		t.Fatalf("MaxOpenConnections = %d, want 3", stats.MaxOpenConnections)
	}
	if _, ok := db.ConnPool.(*gorm.PreparedStmtDB); ok {
		t.Fatal("Open must not install GORM's cross-request prepared statement cache")
	}
}

func TestEnforceTelegramBindingOneToOneCleansDuplicatesAndAddsIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.TelegramBinding{}); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().Add(-time.Hour)
	rows := []model.TelegramBinding{
		{TelegramUserID: 10001, ChatID: 10001, UserID: "user-1"},
		{TelegramUserID: 10002, ChatID: 10002, UserID: "user-1"},
	}
	for i := range rows {
		rows[i].CreatedAt = createdAt.Add(time.Duration(i) * time.Minute)
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := enforceTelegramBindingOneToOne(db); err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := db.Model(&model.TelegramBinding{}).Where("user_id = ?", "user-1").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("active bindings for user-1 = %d, want 1", count)
	}
	var kept model.TelegramBinding
	if err := db.First(&kept, "user_id = ?", "user-1").Error; err != nil {
		t.Fatal(err)
	}
	if kept.TelegramUserID != 10002 {
		t.Fatalf("kept telegram binding = %d, want newest 10002", kept.TelegramUserID)
	}
	if err := db.Create(&model.TelegramBinding{TelegramUserID: 10003, ChatID: 10003, UserID: "user-1"}).Error; err == nil {
		t.Fatal("expected unique index to reject another active binding for the same user")
	}
}

func TestEnsurePerformanceIndexesCreatesHotPathIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}, &model.Favorite{}, &model.PlaybackHistory{}, &model.PlayProfile{}, &model.RefreshToken{}); err != nil {
		t.Fatal(err)
	}
	if err := ensurePerformanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"idx_media_library_created_active",
		"idx_media_library_episode_active",
		"idx_favorites_user_media_active",
		"idx_playback_histories_user_media_active",
		"idx_play_profiles_user_created_active",
		"idx_refresh_tokens_user_active_created",
	} {
		var count int
		if err := db.Raw(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'index' AND name = ?`, name).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("index %s count = %d, want 1", name, count)
		}
	}
}

func TestAutoMigrateBackfillsDeletedSubscriptionsIntoHistoryIdempotently(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Subscription{}); err != nil {
		t.Fatal(err)
	}
	sub := model.Subscription{Name: "凡人修仙传", FeedURL: "resource-import://pansou", Enabled: true}
	if err := db.Create(&sub).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&sub).Error; err != nil {
		t.Fatal(err)
	}

	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var first model.Subscription
	if err := db.Unscoped().First(&first, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if first.ArchivedAt == nil || !first.ArchivedAt.Equal(first.DeletedAt.Time) || first.ArchiveReason != "手动删除" || first.Enabled {
		t.Fatalf("backfilled subscription = %+v", first)
	}
	updatedAt := first.UpdatedAt

	if err := AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	var second model.Subscription
	if err := db.Unscoped().First(&second, "id = ?", sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !second.UpdatedAt.Equal(updatedAt) || second.ArchivedAt == nil || !second.ArchivedAt.Equal(*first.ArchivedAt) {
		t.Fatalf("second migration changed the archived row: before=%+v after=%+v", first, second)
	}
}

func TestEnsureMediaSearchIndexCreatesVersionedTriggers(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Media{}); err != nil {
		t.Fatal(err)
	}
	if err := ensureMediaSearchIndex(db); err != nil {
		t.Fatal(err)
	}
	if !sqliteFTSTableExists(t, db, "media_search_fts") {
		t.Skip("SQLite FTS5 is unavailable in this build")
	}
	var version int
	if err := db.Raw(`SELECT version FROM media_search_meta WHERE id = 1`).Scan(&version).Error; err != nil {
		t.Fatal(err)
	}
	if version != mediaSearchIndexSchemaVersion {
		t.Fatalf("media search schema version = %d, want %d", version, mediaSearchIndexSchemaVersion)
	}
	for _, trigger := range []string{"media_search_fts_ai", "media_search_fts_au", "media_search_fts_ad"} {
		var count int
		if err := db.Raw(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'trigger' AND name = ?`, trigger).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("trigger %s count = %d, want 1", trigger, count)
		}
	}
	media := model.Media{LibraryID: "lib-1", Title: "中文搜索电影", Path: "/media/movie.mkv", Genres: "动画,冒险"}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	var indexed int
	if err := db.Raw(`SELECT COUNT(1) FROM media_search_fts WHERE media_id = ?`, media.ID).Scan(&indexed).Error; err != nil {
		t.Fatal(err)
	}
	if indexed != 1 {
		t.Fatalf("indexed rows = %d, want inserted media indexed", indexed)
	}
}

func sqliteFTSTableExists(t *testing.T, db *gorm.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.Raw(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count == 1
}

func TestCopyModelTablesMigratesExistingSQLiteRows(t *testing.T) {
	src, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, db := range []*gorm.DB{src, dst} {
		if err := db.AutoMigrate(&model.User{}, &model.Library{}, &model.Setting{}); err != nil {
			t.Fatal(err)
		}
	}
	user := model.User{Username: "admin", PasswordHash: "hash", Role: "admin", IsActive: true}
	if err := src.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "Movies", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := src.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	if err := src.Create(&model.Setting{Key: "organize.auto", Value: "false"}).Error; err != nil {
		t.Fatal(err)
	}

	copied, err := copyModelTables(src, dst, 2)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 3 {
		t.Fatalf("copied rows = %d, want 3", copied)
	}
	var got model.User
	if err := dst.First(&got, "username = ?", "admin").Error; err != nil {
		t.Fatal(err)
	}
	if got.ID != user.ID || got.Role != "admin" {
		t.Fatalf("user not preserved: %#v", got)
	}
}

func TestCopyModelTablesResumesPartialSQLiteMigration(t *testing.T) {
	src, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	dst, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, db := range []*gorm.DB{src, dst} {
		if err := db.AutoMigrate(&model.User{}, &model.Media{}, &model.Setting{}); err != nil {
			t.Fatal(err)
		}
	}
	user := model.User{Username: "admin", PasswordHash: "hash", Role: "admin", IsActive: true}
	if err := src.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := dst.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    "library-1",
		Title:        "Resume Migration",
		Path:         "/media/resume.mp4",
		Container:    "mov,mp4,m4a,3gp,3g2,mj2",
		ScrapeStatus: "matched",
		OriginalName: "Resume Migration",
		PosterURL:    "/media/poster.jpg",
		BackdropURL:  "/media/backdrop.jpg",
		VideoCodec:   "hevc",
		AudioCodec:   "eac3",
		DurationSec:  120,
		Genres:       "家庭,动画,冒险,喜剧,奇幻,Peter Del Vecho,Jeff Draheim,詹妮弗·李,克里斯·巴克,伊迪娜·门泽尔,克里斯汀·贝尔,乔什·盖德,乔纳森·格罗夫,埃文·蕾切尔·伍德,斯特林·K·布朗",
		SizeBytes:    1024,
		Width:        3840,
		Height:       2160,
		SeasonNum:    1,
		EpisodeNum:   1,
	}
	if err := src.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := src.Create(&model.Setting{Key: "organize.auto", Value: "false"}).Error; err != nil {
		t.Fatal(err)
	}

	copied, err := copyModelTables(src, dst, 2)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 2 {
		t.Fatalf("copied rows = %d, want 2", copied)
	}
	var got model.Media
	if err := dst.First(&got, "path = ?", media.Path).Error; err != nil {
		t.Fatal(err)
	}
	if got.Container != media.Container {
		t.Fatalf("container = %q, want %q", got.Container, media.Container)
	}
	if got.Genres != media.Genres {
		t.Fatalf("genres = %q, want %q", got.Genres, media.Genres)
	}

	copied, err = copyModelTables(src, dst, 2)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 0 {
		t.Fatalf("second copy rows = %d, want 0", copied)
	}
}

func TestSQLiteMigrationCompleteMarker(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}
	complete, err := sqliteMigrationMarkedComplete(db)
	if err != nil {
		t.Fatal(err)
	}
	if complete {
		t.Fatal("fresh database should not be marked migrated")
	}
	if err := markSQLiteMigrationComplete(db); err != nil {
		t.Fatal(err)
	}
	complete, err = sqliteMigrationMarkedComplete(db)
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("database should be marked migrated")
	}
}

func TestSQLiteMigrationFallsBackToDataDirDefaultPath(t *testing.T) {
	dir := t.TempDir()
	sqlitePath := filepath.Join(dir, "mediastation.db")
	src, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := src.AutoMigrate(&model.User{}, &model.Library{}); err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "real-admin", PasswordHash: "hash", Role: "admin", IsActive: true}
	if err := src.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "Movies", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := src.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := src.DB()
	_ = sqlDB.Close()

	dst, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := dst.AutoMigrate(&model.User{}, &model.Library{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	if err := dst.Create(&model.User{Username: "admin", PasswordHash: "bootstrap", Role: "admin", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.App.DataDir = dir
	cfg.Database.DBPath = filepath.Join(dir, "disabled-sqlite-migration.db")
	sourcePath, err := sqliteMigrationSourcePath(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sourcePath != sqlitePath {
		t.Fatalf("source path = %q, want fallback %q", sourcePath, sqlitePath)
	}

	src2, err := gorm.Open(sqlite.Open(sourcePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB2, _ := src2.DB()
	defer func() {
		if sqlDB2 != nil {
			_ = sqlDB2.Close()
		}
	}()
	if err := resetBootstrapTargetBeforeSQLiteMigrationIfSafe(src2, dst, nil); err != nil {
		t.Fatal(err)
	}
	copied, err := copyModelTables(src2, dst, 2)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 2 {
		t.Fatalf("copied rows = %d, want 2", copied)
	}

	var userCount int64
	if err := dst.Model(&model.User{}).Count(&userCount).Error; err != nil {
		t.Fatal(err)
	}
	if userCount != 1 {
		t.Fatalf("user count = %d, want migrated source only", userCount)
	}
	var got model.User
	if err := dst.First(&got, "username = ?", "real-admin").Error; err != nil {
		t.Fatal(err)
	}
	var libCount int64
	if err := dst.Model(&model.Library{}).Where("path = ?", "/media/movies").Count(&libCount).Error; err != nil {
		t.Fatal(err)
	}
	if libCount != 1 {
		t.Fatalf("library count = %d, want 1", libCount)
	}
}

func TestOpenSQLiteMigrationSourceUsesFallbackSourcePath(t *testing.T) {
	dir := t.TempDir()
	sqlitePath := filepath.Join(dir, "mediastation.db")
	src, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := src.AutoMigrate(&model.User{}, &model.Library{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}
	if err := src.Create(&model.User{Username: "real-admin", PasswordHash: "hash", Role: "admin", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := src.Create(&model.Library{Name: "Movies", Path: "/media/movies", Type: "movie", Enabled: true}).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := src.DB()
	_ = sqlDB.Close()

	dst, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := dst.AutoMigrate(&model.User{}, &model.Library{}, &model.Setting{}); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.App.DataDir = dir
	cfg.Database.DBPath = filepath.Join(dir, "disabled-sqlite-migration.db")
	sourcePath, err := sqliteMigrationSourcePath(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sourcePath != sqlitePath {
		t.Fatalf("source path = %q, want %q", sourcePath, sqlitePath)
	}

	src2, err := openSQLiteMigrationSource(cfg, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	sqlDB2, _ := src2.DB()
	defer func() {
		if sqlDB2 != nil {
			_ = sqlDB2.Close()
		}
	}()
	copied, err := copyModelTables(src2, dst, 2)
	if err != nil {
		t.Fatal(err)
	}
	if copied != 2 {
		t.Fatalf("copied rows = %d, want 2", copied)
	}
	var userCount int64
	if err := dst.Model(&model.User{}).Where("username = ?", "real-admin").Count(&userCount).Error; err != nil {
		t.Fatal(err)
	}
	if userCount != 1 {
		t.Fatalf("migrated user count = %d, want 1", userCount)
	}
}
