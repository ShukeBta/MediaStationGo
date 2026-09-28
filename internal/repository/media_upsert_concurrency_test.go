package repository

import (
	"errors"
	"sync/atomic"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestMediaUpsertConcurrentInsertReusesWinnerWithinTransaction(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Media{}); err != nil {
		t.Fatal(err)
	}
	var injected atomic.Bool
	err = db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Callback().Query().After("gorm:query").Register("test:concurrent_insert", func(query *gorm.DB) {
			if query.Statement.Table != "media" || !errors.Is(query.Error, gorm.ErrRecordNotFound) || !injected.CompareAndSwap(false, true) {
				return
			}
			winner := model.Media{Base: model.Base{ID: "winner"}, Title: "Manual metadata", Path: "/movies/race.mkv", ScrapeStatus: "matched", TMDbID: 42}
			if err := tx.Create(&winner).Error; err != nil {
				query.AddError(err)
			}
		}); err != nil {
			return err
		}
		defer tx.Callback().Query().Remove("test:concurrent_insert")
		incoming := model.Media{Title: "Scanner filename", Path: "/movies/race.mkv", SizeBytes: 1024}
		if err := New(tx).Media.Upsert(t.Context(), &incoming); err != nil {
			return err
		}
		if incoming.ID != "winner" || incoming.Title != "Manual metadata" || incoming.TMDbID != 42 {
			t.Fatalf("winner lost: %#v", incoming)
		}
		return tx.Create(&model.Media{Title: "Transaction continues", Path: "/movies/next.mkv"}).Error
	})
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	db.Model(&model.Media{}).Count(&count)
	if count != 2 {
		t.Fatalf("transaction aborted or duplicate inserted: %d", count)
	}
}
