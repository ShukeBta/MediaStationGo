package service

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestWorkSearchSeparatesFlatLibraryFilms(t *testing.T) {
	testWorkSearchSeparatesFlatLibraryFilms(t, func(t *testing.T) *gorm.DB {
		return newServiceTestDB(t, &model.Library{}, &model.Media{})
	})
}

func TestPostgresWorkSearchSeparatesFlatLibraryFilms(t *testing.T) {
	db, _ := postgresBackupTestDatabase(t)
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	testWorkSearchSeparatesFlatLibraryFilms(t, func(t *testing.T) *gorm.DB {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatal(tx.Error)
		}
		t.Cleanup(func() { tx.Rollback() })
		return tx
	})
}

func testWorkSearchSeparatesFlatLibraryFilms(t *testing.T, newDB func(*testing.T) *gorm.DB) {
	t.Helper()
	for _, legacy := range []bool{false, true} {
		name := "missing-keys"
		if legacy {
			name = "legacy-directory-key"
		}
		t.Run(name, func(t *testing.T) {
			db := newDB(t)
			repos := repository.New(db)
			lib := model.Library{Base: model.Base{ID: "flat-search"}, Name: "My Cinema", Path: "/vault/My Cinema", Type: "movie", Enabled: true}
			hiddenLib := model.Library{Base: model.Base{ID: "hidden-search"}, Name: "Hidden Cinema", Path: "/vault/Hidden Cinema", Type: "movie", Enabled: true}
			if err := db.Create(&[]model.Library{lib, hiddenLib}).Error; err != nil {
				t.Fatal(err)
			}
			now := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
			rows := []model.Media{
				{Base: model.Base{ID: "target-a", CreatedAt: now.Add(-3 * time.Hour)}, LibraryID: lib.ID, Title: "Target Film", Path: lib.Path + "/target-a.mkv", Year: 2020, Actors: "NeedleActor Literal_100%"},
				{Base: model.Base{ID: "target-z", CreatedAt: now.Add(-2 * time.Hour)}, LibraryID: lib.ID, Title: "Target Film", Path: lib.Path + "/target-z.mkv", Year: 2020, PosterURL: "/art/poster.jpg"},
				{Base: model.Base{ID: "other", CreatedAt: now}, LibraryID: lib.ID, Title: "Other Film", Path: lib.Path + "/other.mkv", Year: 2021, Actors: "LiteralX100Z"},
				{Base: model.Base{ID: "adult", CreatedAt: now}, LibraryID: lib.ID, Title: "Adult Secret Film", Path: lib.Path + "/adult.mkv", Year: 2020, NSFW: true, Actors: "NeedleActor Literal_100%"},
				{Base: model.Base{ID: "hidden", CreatedAt: now}, LibraryID: hiddenLib.ID, Title: "Hidden Secret Film", Path: hiddenLib.Path + "/hidden.mkv", Year: 2020, Actors: "NeedleActor Literal_100%"},
			}
			if legacy {
				for i := range rows {
					rows[i].SeriesKey = MediaSeriesKey(rows[i])
					rows[i].SeriesKeyVersion = 1
				}
			}
			if err := db.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}
			svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
			visibility := MediaVisibility{AllowedLibraryIDs: []string{lib.ID}}
			for _, query := range []string{"Target", "Film 2020", "NeedleActor", "needleactor,2020"} {
				cards, total, err := svc.SearchMediaVisibleSeriesPage(t.Context(), query, 1, 1, visibility)
				if err != nil || total != 1 || len(cards) != 1 || cards[0].Rep.ID != "target-z" || cards[0].Count != 2 {
					t.Fatalf("query %q must retain both target versions and exclude unrelated/hidden works: cards=%#v total=%d err=%v", query, cards, total, err)
				}
				empty, sameTotal, err := svc.SearchMediaVisibleSeriesPage(t.Context(), query, 2, 1, visibility)
				if err != nil || sameTotal != 1 || len(empty) != 0 {
					t.Fatalf("query %q page 2: cards=%#v total=%d err=%v", query, empty, sameTotal, err)
				}
			}
			for page, id := range []string{"other", "target-z"} {
				cards, total, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "Film", page+1, 1, visibility)
				if err != nil || total != 2 || len(cards) != 1 || cards[0].Rep.ID != id {
					t.Fatalf("all-film page %d must page public works: cards=%#v total=%d err=%v", page+1, cards, total, err)
				}
			}
			cards, total, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "Secret", 1, 20, visibility)
			if err != nil || total != 0 || len(cards) != 0 {
				t.Fatalf("hidden works matched: cards=%#v total=%d err=%v", cards, total, err)
			}
			cards, total, err = svc.SearchMediaVisibleSeriesPage(t.Context(), "%_", 1, 20, visibility)
			if err != nil || total != 0 || len(cards) != 0 {
				t.Fatalf("punctuation-only query must not become an unrestricted LIKE: cards=%#v total=%d err=%v", cards, total, err)
			}
			cards, total, err = svc.SearchMediaVisibleSeriesPage(t.Context(), "Literal_100%", 1, 20, visibility)
			if err != nil || total != 2 || len(cards) != 2 {
				t.Fatalf("existing punctuation tokenization/LIKE matching changed: cards=%#v total=%d err=%v", cards, total, err)
			}
			cards, total, err = svc.SearchMediaWorkCandidatesVisible(t.Context(), []string{"Film 2020", "Other"}, lib.ID, 1, visibility)
			if err != nil || total != 2 || len(cards) != 1 {
				t.Fatalf("ingest candidate OR queries/limit changed: cards=%#v total=%d err=%v", cards, total, err)
			}

			// An older exact match must stay ahead of a newer prefix match even
			// when both were originally collapsed into the same physical group.
			if err := db.Model(&model.Media{}).Where("id = ?", "other").Update("title", "Target Film Returns").Error; err != nil {
				t.Fatal(err)
			}
			cards, total, err = svc.SearchMediaVisibleSeriesPage(t.Context(), "Target Film", 1, 1, visibility)
			if err != nil || total != 2 || len(cards) != 1 || cards[0].Rep.ID != "target-z" || cards[0].Count != 2 {
				t.Fatalf("exact match ranking lost after physical split: cards=%#v total=%d err=%v", cards, total, err)
			}
			cards, total, err = svc.SearchMediaVisibleSeriesPage(t.Context(), "Target Film", 2, 1, visibility)
			if err != nil || total != 2 || len(cards) != 1 || cards[0].Rep.ID != "other" {
				t.Fatalf("prefix match second page: cards=%#v total=%d err=%v", cards, total, err)
			}
		})
	}
}
