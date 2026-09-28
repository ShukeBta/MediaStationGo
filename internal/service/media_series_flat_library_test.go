package service

import (
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestPersistedSeriesCardsSeparateFlatLibraryFilms(t *testing.T) {
	testPersistedSeriesCardsSeparateFlatLibraryFilms(t, func(t *testing.T) *gorm.DB {
		return newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.WeeklyFeaturedSelection{})
	})
}

func TestPostgresPersistedSeriesCardsSeparateFlatLibraryFilms(t *testing.T) {
	db, _ := postgresBackupTestDatabase(t)
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}, &model.WeeklyFeaturedSelection{}); err != nil {
		t.Fatal(err)
	}
	testPersistedSeriesCardsSeparateFlatLibraryFilms(t, func(t *testing.T) *gorm.DB {
		// Each case uses the same disposable database with a transaction that
		// rolls back its rows before the next case starts.
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		t.Cleanup(func() { tx.Rollback() })
		return tx
	})
}

func testPersistedSeriesCardsSeparateFlatLibraryFilms(t *testing.T, newDB func(*testing.T) *gorm.DB) {
	t.Helper()
	for _, legacyKeys := range []bool{false, true} {
		name := "missing-keys"
		if legacyKeys {
			name = "legacy-directory-keys"
		}
		t.Run(name, func(t *testing.T) {
			db := newDB(t)
			repos := repository.New(db)
			lib := model.Library{Base: model.Base{ID: "flat-library"}, Name: "My Cinema", Path: "/vault/My Cinema", Type: "movie", Enabled: true}
			if err := db.Create(&lib).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
			rows := []model.Media{
				{Base: model.Base{ID: "film-a", CreatedAt: now.Add(-time.Hour)}, LibraryID: lib.ID, Title: "First Film", Path: lib.Path + "/first.mkv", Rating: 9},
				{Base: model.Base{ID: "film-z", CreatedAt: now}, LibraryID: lib.ID, Title: "Second Film", Path: lib.Path + "/second.mkv", Rating: 6},
			}
			if legacyKeys {
				for i := range rows {
					// Older writes calculated keys without the library path and
					// collapsed unrelated films directly under a custom root.
					rows[i].SeriesKey = MediaSeriesKey(rows[i])
					rows[i].SeriesKeyVersion = 1
				}
			}
			if err := db.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}
			svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
			visibility := MediaVisibility{IncludeNSFW: true, AllowedLibraryIDs: []string{lib.ID}}
			recent, err := svc.ListRecentSeriesCards(t.Context(), 2, visibility)
			if err != nil {
				t.Fatal(err)
			}
			if len(recent) != 2 || recent[0].Rep.ID != "film-z" || recent[1].Rep.ID != "film-a" || recent[0].Count != 1 || recent[1].Count != 1 {
				t.Fatalf("flat library must expose two separate recent films: %#v", recent)
			}
			if recent[0].Key == recent[1].Key {
				t.Fatal("unrelated films share a public key")
			}
			for page := 1; page <= 2; page++ {
				cards, total, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, page, 1, visibility)
				if err != nil || total != 2 || len(cards) != 1 || cards[0].Key != recent[page-1].Key {
					t.Fatalf("page %d: cards=%#v total=%d err=%v", page, cards, total, err)
				}
			}
			for _, card := range recent {
				items, err := svc.ListLibrarySeriesEpisodes(t.Context(), lib.ID, card.Key, visibility)
				if err != nil || len(items) != 1 || items[0].ID != card.Rep.ID {
					t.Fatalf("card %q detail mismatch: %#v err=%v", card.Key, items, err)
				}
			}
			featured, _, err := svc.WeeklyFeaturedCard(t.Context(), "viewer", now, visibility)
			if err != nil || featured == nil || featured.Rep.ID != "film-a" || featured.Count != 1 {
				t.Fatalf("featured must use each film's own rating: %#v err=%v", featured, err)
			}
		})
	}
}
