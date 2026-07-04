package database

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func resetBootstrapTargetBeforeSQLiteMigrationIfSafe(src, target *gorm.DB, log *zap.Logger) error {
	hasRows, err := sqliteSourceHasMigratableRows(src)
	if err != nil {
		return err
	}
	if !hasRows {
		return nil
	}
	bootstrapOnly, err := targetLooksLikeBootstrapOnly(target)
	if err != nil || !bootstrapOnly {
		return err
	}
	for i := len(model.AllModels()) - 1; i >= 0; i-- {
		m := model.AllModels()[i]
		if !target.Migrator().HasTable(m) {
			continue
		}
		if err := target.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(m).Error; err != nil {
			return fmt.Errorf("clear bootstrap target table %T: %w", m, err)
		}
	}
	if log != nil {
		log.Warn("cleared bootstrap postgres rows before sqlite migration")
	}
	return nil
}

func sqliteSourceHasMigratableRows(src *gorm.DB) (bool, error) {
	for _, table := range []string{"users", "libraries", "media", "settings"} {
		exists, err := sqliteTableExists(src, table)
		if err != nil {
			return false, err
		}
		if !exists {
			continue
		}
		var count int64
		if err := src.Raw("SELECT COUNT(1) FROM " + quoteIdent(table)).Scan(&count).Error; err != nil {
			return false, fmt.Errorf("count sqlite table %s: %w", table, err)
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func targetLooksLikeBootstrapOnly(target *gorm.DB) (bool, error) {
	for _, m := range []any{
		&model.Library{},
		&model.Series{},
		&model.Media{},
		&model.PlaybackHistory{},
		&model.Favorite{},
		&model.Playlist{},
		&model.PlaylistItem{},
		&model.DownloadTask{},
		&model.Subscription{},
	} {
		if !target.Migrator().HasTable(m) {
			continue
		}
		var count int64
		if err := target.Unscoped().Model(m).Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return false, nil
		}
	}

	var userCount int64
	if !target.Migrator().HasTable(&model.User{}) {
		return true, nil
	}
	if err := target.Model(&model.User{}).Count(&userCount).Error; err != nil {
		return false, err
	}
	if userCount == 0 {
		return true, nil
	}
	if userCount != 1 {
		return false, nil
	}
	var user model.User
	if err := target.Unscoped().Where("username = ?", "admin").First(&user).Error; err != nil {
		return false, nil
	}
	return user.Role == "admin", nil
}

func sqliteMigrationMarkedComplete(db *gorm.DB) (bool, error) {
	var value string
	err := db.Raw("SELECT value FROM "+quoteIdent("settings")+" WHERE "+quoteIdent("key")+" = ?", sqliteMigrationCompleteSettingKey).Scan(&value).Error
	if err != nil {
		return false, fmt.Errorf("check sqlite migration marker: %w", err)
	}
	return strings.EqualFold(strings.TrimSpace(value), "true"), nil
}

func markSQLiteMigrationComplete(db *gorm.DB) error {
	now := time.Now()
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&model.Setting{
		Key:       sqliteMigrationCompleteSettingKey,
		Value:     "true",
		UpdatedAt: now,
	}).Error; err != nil {
		return fmt.Errorf("mark sqlite migration complete: %w", err)
	}
	return nil
}
