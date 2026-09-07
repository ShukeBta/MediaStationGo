package service

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"gorm.io/gorm"
)

func TestEmbySQLLatestOnlyLoadsSelectedGroups(t *testing.T) {
	e, lib := embyProjectionFixture(t, 17, 3)
	if _, err := e.repo.Media.BackfillEmbyKeys(t.Context(), 500); err != nil {
		t.Fatal(err)
	}
	var loaded int64
	if err := e.repo.DB.Callback().Query().After("gorm:query").Register("sql_latest_rows", func(db *gorm.DB) {
		if db.Statement.Table == "media" && len(db.Statement.Selects) == 0 {
			loaded += db.RowsAffected
		}
	}); err != nil {
		t.Fatal(err)
	}
	items, err := e.latestSeriesItemsForLibrary(t.Context(), "", lib.ID, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || loaded != 6 {
		t.Fatalf("items=%d full rows=%d; want 2 and 6", len(items), loaded)
	}
}

func TestEmbySQLLatestRequiresInitialMigration(t *testing.T) {
	e, lib := embyProjectionFixture(t, 2, 251)
	if _, err := e.latestSeriesItemsForLibrary(t.Context(), "", lib.ID, 2); err == nil {
		t.Fatal("unmigrated inventory silently served via old full-library grouping")
	}
	for {
		n, err := e.repo.Media.BackfillEmbyKeys(t.Context(), 500)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			break
		}
	}
	items, err := e.latestSeriesItemsForLibrary(t.Context(), "", lib.ID, 2)
	if err != nil || len(items) != 2 {
		t.Fatalf("after migration items=%d err=%v", len(items), err)
	}
}

func TestEmbyPersistedKeysLifecycle(t *testing.T) {
	e, lib := embyProjectionFixture(t, 0, 0)
	if err := database.AutoMigrate(e.repo.DB); err != nil {
		t.Fatal(err)
	}
	e.repo.Media.SetSeriesKeyFunc(MediaSeriesKey)
	row := model.Media{LibraryID: lib.ID, Title: "Example S01E01", Path: "/media/example/e1.mkv", SeasonNum: 1, EpisodeNum: 1}
	if err := e.repo.Media.Upsert(t.Context(), &row); err != nil {
		t.Fatal(err)
	}
	check := func() model.Media {
		t.Helper()
		var stored model.Media
		if err := e.repo.DB.First(&stored, "id = ?", row.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.EmbyKeyVersion != repository.EmbyKeyVersion || stored.EmbySeriesKey != e.seriesIDForMedia(&stored) {
			t.Fatalf("stale key: %#v", stored)
		}
		return stored
	}
	before := check()
	if before.SeriesID != "" {
		t.Fatal("public identity was written into SeriesID")
	}
	if err := e.repo.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, row.ID, map[string]any{"library_id": "target", "path": "/media/target/e1.mkv"}); err != nil {
		t.Fatal(err)
	}
	moved := check()
	if moved.EmbySeriesKey == before.EmbySeriesKey {
		t.Fatal("virtual identity did not follow its existing library rule")
	}
	if err := e.repo.DB.Model(&model.Media{}).Where("id = ?", row.ID).Updates(map[string]any{"title": "Renamed S01E01", "part_group_key": "part"}).Error; err != nil {
		t.Fatal(err)
	}
	var dirty model.Media
	if err := e.repo.DB.First(&dirty, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if dirty.EmbyKeyVersion != 0 {
		t.Fatal("direct writer did not invalidate key")
	}
	if n, err := e.repo.Media.BackfillEmbyKeys(t.Context(), 500); err != nil || n != 1 {
		t.Fatalf("repair n=%d err=%v", n, err)
	}
	repaired := check()
	if repaired.EmbyListKey != multipartSeriesID("target", "part") {
		t.Fatal("multipart identity changed")
	}
	if n, err := e.repo.Media.BackfillEmbyKeys(t.Context(), 500); err != nil || n != 0 {
		t.Fatalf("non-idempotent repair n=%d err=%v", n, err)
	}
	if err := database.AutoMigrate(e.repo.DB); err != nil {
		t.Fatal(err)
	}
	check()
}

func TestEmbySQLLatestMatchesReferenceAfterMetadataChange(t *testing.T) {
	e, lib := embyProjectionFixture(t, 17, 3)
	e.repo.Media.SetSeriesKeyFunc(MediaSeriesKey)
	for _, field := range []string{"poster_url", "title"} {
		if err := e.repo.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, "work-0000-episode-000", map[string]any{field: "Changed"}); err != nil {
			t.Fatal(err)
		}
		var rows []model.Media
		if err := e.repo.DB.Where("library_id = ?", lib.ID).Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		groups, err := e.seriesGroupsFromMedia(t.Context(), rows)
		if err != nil {
			t.Fatal(err)
		}
		sortSeriesGroups(groups, ItemsParams{SortBy: "DateCreated", SortOrder: "Descending"})
		want := make([]map[string]any, 0, 12)
		for _, group := range groups[:12] {
			want = append(want, e.seriesPayload(group))
		}
		got, err := e.latestSeriesItemsForLibrary(t.Context(), "", lib.ID, 12)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%s update changed SQL result", field)
		}
	}
}

func BenchmarkEmbySQLLatest(b *testing.B) {
	for _, works := range []int{177, 1770} {
		b.Run(fmt.Sprintf("works_%d", works), func(b *testing.B) {
			e, lib := embyProjectionFixture(b, works, 5)
			for {
				n, err := e.repo.Media.BackfillEmbyKeys(b.Context(), 1000)
				if err != nil {
					b.Fatal(err)
				}
				if n == 0 {
					break
				}
			}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := e.latestSeriesItemsForLibrary(b.Context(), "", lib.ID, 15); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkEmbySQLBrowseScaling(b *testing.B) {
	for _, works := range []int{177, 1770} {
		b.Run(fmt.Sprintf("works_%d_episodes_%d", works, works*55), func(b *testing.B) {
			e, lib := embyProjectionFixture(b, works, 55)
			if _, err := e.InitializeBrowseKeys(b.Context()); err != nil {
				b.Fatal(err)
			}
			for _, pageSize := range []int{12, 48} {
				b.Run(fmt.Sprintf("page_%d", pageSize), func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if _, err := e.seriesItemsForLibrary(b.Context(), lib.ID, ItemsParams{Limit: pageSize, SortBy: "DateLastContentAdded,SortName", SortOrder: "Descending,Ascending"}); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}
