package service

import (
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
	"strings"
	"testing"
)

func TestEmbyLibraryPageDoesNotResolveLibraryAsSeries(t *testing.T) {
	e, lib := embyProjectionFixture(t, 2, 3)
	if _, err := e.InitializeBrowseKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	checks := 0
	if err := e.repo.DB.Callback().Query().After("gorm:query").Register("library_identity_checks", func(db *gorm.DB) {
		if strings.Contains(db.Statement.SQL.String(), "emby_config_key") && strings.Contains(db.Statement.SQL.String(), "LIMIT 501") {
			checks++
		}
	}); err != nil {
		t.Fatal(err)
	}
	page, err := e.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Series"}, Recursive: true, Limit: 48})
	if err != nil || page["TotalRecordCount"] != 2 || checks != 1 {
		t.Fatalf("page=%v checks=%d err=%v", page, checks, err)
	}
}

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
	e.seriesPayload(season.Series)
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

func TestEmbyShowEpisodesKeepsParentAndAvoidsSeasonInventory(t *testing.T) {
	e, _ := embyProjectionFixture(t, 17, 3)
	if _, err := e.InitializeBrowseKeys(t.Context()); err != nil {
		t.Fatal(err)
	}
	var source model.Media
	if err := e.repo.DB.First(&source, "id = ?", "work-0000-episode-000").Error; err != nil {
		t.Fatal(err)
	}
	checks, identityScans := 0, 0
	if err := e.repo.DB.Callback().Query().After("gorm:query").Register("show_episode_scope", func(db *gorm.DB) {
		sql := db.Statement.SQL.String()
		if strings.Contains(sql, "emby_config_key") && strings.Contains(sql, "LIMIT 501") {
			checks++
			if strings.Contains(sql, "id IN (SELECT") {
				t.Error("projection check retained full-scope self join")
			}
		}
		if strings.Contains(sql, "SELECT DISTINCT") && strings.Contains(sql, "AS group_key") {
			identityScans++
		}
	}); err != nil {
		t.Fatal(err)
	}
	p := ItemsParams{ShowID: source.EmbySeriesKey, ParentID: seasonID(source.EmbySeriesKey, 1), IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 1, StartIndex: 1, OmitMediaSources: true}
	page, err := e.Items(t.Context(), p)
	if err != nil || page["TotalRecordCount"] != 3 || len(page["Items"].([]map[string]any)) != 1 {
		t.Fatalf("page=%v err=%v", page, err)
	}
	if checks != 1 || identityScans != 0 {
		t.Fatalf("checks=%d identity scans=%d; want 1,0", checks, identityScans)
	}
	// A direct insert without projections must still join an already-known
	// show; restricting repair by the old projected key would omit this row.
	source.ID = "new-unprojected-episode"
	source.Path += ".new"
	source.EpisodeNum = 4
	source.EmbySeriesKey, source.EmbyListKey, source.EmbyConfigKey = "", "", ""
	source.EmbyKeyVersion = 0
	if err := e.repo.DB.Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	page, err = e.Items(t.Context(), p)
	if err != nil || page["TotalRecordCount"] != 4 {
		t.Fatalf("new episode missing from known show: %v %v", page, err)
	}
	// A valid season does not override a different authoritative show.
	p.ShowID = "missing-show"
	page, err = e.Items(t.Context(), p)
	if err != nil || len(page["Items"].([]map[string]any)) != 0 {
		t.Fatalf("foreign season leaked episodes: %v %v", page, err)
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
