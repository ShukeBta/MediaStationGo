package database

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestSubscriptionIdentityMigrationRekeysExistingPTSeason(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Subscription{}); err != nil {
		t.Fatal(err)
	}
	row := model.Subscription{UserID: "u1", Name: "Show", FeedURL: "site-search://resources?keyword=Show", DeliveryMode: "download"}
	model.RefreshSubscriptionIdentity(&row)
	// Simulate an existing S2 row created before season entered the identity.
	row.SeasonNumber = 2
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureSubscriptionIdentityUniqueness(db); err != nil {
		t.Fatal(err)
	}
	var stored model.Subscription
	if err := db.First(&stored, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.IdentityKey == row.IdentityKey || stored.IdentityKey != model.SubscriptionIdentityKey(&stored) || stored.ArchivedAt != nil {
		t.Fatal("existing S2 rule was not rekeyed while remaining active")
	}
	seasonOne := stored
	seasonOne.ID = ""
	seasonOne.SeasonNumber = 1
	model.RefreshSubscriptionIdentity(&seasonOne)
	if err := db.Create(&seasonOne).Error; err != nil {
		t.Fatalf("another season should be allowed after migration: %v", err)
	}
	if err := ensureSubscriptionIdentityUniqueness(db); err != nil {
		t.Fatalf("repeated migration should be safe: %v", err)
	}
}
