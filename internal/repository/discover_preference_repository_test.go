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

func TestDiscoverPreferenceUpgradeDoesNotOverwriteConcurrentSelection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.UserDiscoverPreference{}); err != nil {
		t.Fatal(err)
	}
	repo := &DiscoverPreferenceRepository{db: db}
	legacy := &model.UserDiscoverPreference{UserID: "user", SelectedSections: []string{"tmdb_chinese_movie"}}
	if err := repo.Upsert(t.Context(), legacy); err != nil {
		t.Fatal(err)
	}
	explicit := &model.UserDiscoverPreference{UserID: "user", SelectedSections: []string{}, SectionsVersion: model.DiscoverSectionsVersion}
	if err := repo.Upsert(t.Context(), explicit); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpgradeSections(t.Context(), legacy, []string{"tmdb_chinese_movie", "tmdb_chinese_latest_movie"}, model.DiscoverSectionsVersion); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindByUserID(t.Context(), "user")
	if err != nil || got == nil || len(got.SelectedSections) != 0 || got.SectionsVersion != model.DiscoverSectionsVersion {
		t.Fatalf("migration overwrote explicit selection: %+v, %v", got, err)
	}
}
