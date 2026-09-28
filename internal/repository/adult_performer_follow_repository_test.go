package repository

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestAdultPerformerFollowRepositoryIsUserScoped(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AdultPerformerFollow{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := &AdultPerformerFollowRepository{db: db}
	follow := &model.AdultPerformerFollow{
		UserID: "user-1", Name: "Actor", NameKey: "actor", Source: "javdb", SourceID: "BzpA",
	}
	if err := repo.Upsert(t.Context(), follow); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	follow.ImageURL = "https://img.example/actor.jpg"
	if err := repo.Upsert(t.Context(), follow); err != nil {
		t.Fatalf("update: %v", err)
	}
	rows, err := repo.ListByUser(t.Context(), "user-1")
	if err != nil || len(rows) != 1 || rows[0].ImageURL != follow.ImageURL {
		t.Fatalf("rows = %#v err=%v", rows, err)
	}
	if other, err := repo.ListByUser(t.Context(), "user-2"); err != nil || len(other) != 0 {
		t.Fatalf("other user rows = %#v err=%v", other, err)
	}
	if deleted, err := repo.DeleteOwned(t.Context(), "user-2", rows[0].ID); err != nil || deleted {
		t.Fatalf("cross-user delete: deleted=%v err=%v", deleted, err)
	}
	if deleted, err := repo.DeleteOwned(t.Context(), "user-1", rows[0].ID); err != nil || !deleted {
		t.Fatalf("owner delete: deleted=%v err=%v", deleted, err)
	}
	if err := repo.Upsert(t.Context(), follow); err != nil {
		t.Fatalf("re-follow after soft delete: %v", err)
	}
	if restored, err := repo.ListByUser(t.Context(), "user-1"); err != nil || len(restored) != 1 {
		t.Fatalf("restored rows = %#v err=%v", restored, err)
	}
}
