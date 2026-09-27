package repository

import (
	"slices"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestDiscoverPreferenceRepositoryIsUserScopedAndPersistsEmptySelection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserDiscoverPreference{}); err != nil {
		t.Fatal(err)
	}
	repo := &DiscoverPreferenceRepository{db: db}
	preference := &model.UserDiscoverPreference{
		UserID:           "user-1",
		SelectedSections: []string{"tmdb_trending_day", "adult_followed"},
		AdultFD2PPVSort:  "views",
	}
	if err := repo.Upsert(t.Context(), preference); err != nil {
		t.Fatal(err)
	}
	row, err := repo.FindByUserID(t.Context(), "user-1")
	if err != nil || row == nil || !slices.Equal(row.SelectedSections, preference.SelectedSections) || row.AdultFD2PPVSort != "views" {
		t.Fatalf("row = %#v err=%v", row, err)
	}
	if other, err := repo.FindByUserID(t.Context(), "user-2"); err != nil || other != nil {
		t.Fatalf("other = %#v err=%v", other, err)
	}
	preference.SelectedSections = []string{"adult_followed", "tmdb_trending_day"}
	preference.AdultFD2PPVSort = "favorites"
	if err := repo.Upsert(t.Context(), preference); err != nil {
		t.Fatal(err)
	}
	row, err = repo.FindByUserID(t.Context(), "user-1")
	if err != nil || row == nil || !slices.Equal(row.SelectedSections, preference.SelectedSections) || row.AdultFD2PPVSort != "favorites" {
		t.Fatalf("reordered row = %#v err=%v", row, err)
	}
	preference.SelectedSections = []string{}
	if err := repo.Upsert(t.Context(), preference); err != nil {
		t.Fatal(err)
	}
	row, err = repo.FindByUserID(t.Context(), "user-1")
	if err != nil || row == nil || len(row.SelectedSections) != 0 || row.AdultFD2PPVSort != "favorites" {
		t.Fatalf("empty selection row = %#v err=%v", row, err)
	}
}
