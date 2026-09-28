package database

import (
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"testing"
)

func TestMediaProbeCleanupOnSourceChangeAndDelete(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Media{}, &model.MediaProbeMetadata{}); err != nil {
		t.Fatal(err)
	}
	if err := ensureMediaProbeMetadataCleanup(db); err != nil {
		t.Fatal(err)
	}
	if err := ensureMediaProbeMetadataCleanup(db); err != nil {
		t.Fatal(err)
	}
	m := model.Media{Title: "test", Path: "/media/a.mkv"}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"path", "strm_url", "size_bytes", "delete"} {
		row := model.MediaProbeMetadata{MediaID: m.ID, ProbeJSON: "{}", SchemaVersion: 1, SourceKey: "key"}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		var err error
		if field == "delete" {
			err = db.Delete(&m).Error
		} else if field == "size_bytes" {
			err = db.Model(&m).Update(field, 123).Error
		} else {
			err = db.Model(&m).Update(field, "new-source").Error
		}
		if err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := db.Model(&model.MediaProbeMetadata{}).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s left stale document", field)
		}
	}
}
