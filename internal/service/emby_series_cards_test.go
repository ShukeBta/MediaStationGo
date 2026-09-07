package service

import (
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
	"testing"
)

func TestEmbyColdSeasonLoadsOnlyItsSeriesAndReflectsMoves(t *testing.T) {
	e, lib := embyProjectionFixture(t, 17, 3)
	e.repo.Media.SetSeriesKeyFunc(MediaSeriesKey)
	if _, err := e.InitializeBrowseKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	var source model.Media
	if err := e.repo.DB.Where("id = ?", "work-0000-episode-000").First(&source).Error; err != nil {
		t.Fatal(err)
	}
	var loaded int64
	if err := e.repo.DB.Callback().Query().After("gorm:query").Register("card_cold_detail", func(db *gorm.DB) {
		if db.Statement.Table == "media" && len(db.Statement.Selects) == 0 {
			loaded += db.RowsAffected
		}
	}); err != nil {
		t.Fatal(err)
	}
	id := seasonID(source.EmbySeriesKey, 1)
	season, ok, err := e.findSeasonGroup(t.Context(), id, "")
	if err != nil || !ok || len(season.Episodes) != 3 || loaded != 3 {
		t.Fatalf("season ok=%v episodes=%d loaded=%d err=%v", ok, len(season.Episodes), loaded, err)
	}
	e.rememberSeriesGroup(season.Series)
	if _, err := e.repo.Media.UpdateManyWithCurrentSeriesKeys(t.Context(), e.repo.DB, []string{source.ID}, map[string]any{"library_id": "moved-library"}); err != nil {
		t.Fatal(err)
	}
	group, ok, err := e.findSeriesGroup(t.Context(), source.EmbySeriesKey, "")
	if err != nil || !ok || len(group.Episodes) != 2 {
		t.Fatalf("stale detail after move: ok=%v episodes=%d err=%v", ok, len(group.Episodes), err)
	}
	page, err := e.seriesItemsForLibrary(t.Context(), lib.ID, ItemsParams{Limit: 48})
	if err != nil || page["TotalRecordCount"] != 17 {
		t.Fatalf("list after move: %v %v", page, err)
	}
}

func TestEmbyGenresWeightedAggregationKeepsEmptyPageTotal(t *testing.T) {
	e, lib := embyProjectionFixture(t, 3, 5)
	first, err := e.Genres(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	items := first["Items"].([]map[string]any)
	if len(items) == 0 {
		t.Fatal("missing facets")
	}
	for _, item := range items {
		if item["RecursiveItemCount"] != 15 {
			t.Fatalf("incorrect weighted count: %v", item)
		}
	}
	empty, err := e.Genres(t.Context(), ItemsParams{ParentID: lib.ID, StartIndex: 100, Limit: 24})
	if err != nil || empty["TotalRecordCount"] != first["TotalRecordCount"] || len(empty["Items"].([]map[string]any)) != 0 {
		t.Fatalf("empty page=%v err=%v", empty, err)
	}
}
