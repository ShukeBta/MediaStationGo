package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyManualSeriesPreservesSeasonsAndEpisodeVersions(t *testing.T) {
	e := newTestEmbyService(t)
	lib := model.Library{Name: "手动剧集", Path: "/media/shows", Type: "tv", Enabled: true}
	if err := e.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "a-1080"}, LibraryID: lib.ID, Title: "Show", Path: "/media/shows/Show/S02E03.1080.mkv", SeasonNum: 2, EpisodeNum: 3, PartGroupKey: "manual-a", PartIndex: 1},
		{Base: model.Base{ID: "a-2160"}, LibraryID: lib.ID, Title: "Show", Path: "/media/shows/Show/S02E03.2160.mkv", SeasonNum: 2, EpisodeNum: 3, PartGroupKey: "manual-a", PartIndex: 2},
		{Base: model.Base{ID: "a-next"}, LibraryID: lib.ID, Title: "Show", Path: "/media/shows/Show/S02E04.mkv", SeasonNum: 2, EpisodeNum: 4, PartGroupKey: "manual-a", PartIndex: 3},
		{Base: model.Base{ID: "a-special"}, LibraryID: lib.ID, Title: "Show", Path: "/media/shows/Show/S00E01.mkv", SeasonNum: 0, EpisodeNum: 1, PartGroupKey: "manual-a", PartIndex: 4},
		{Base: model.Base{ID: "other-group"}, LibraryID: lib.ID, Title: "Show", Path: "/media/shows/Other/S02E03.mkv", SeasonNum: 2, EpisodeNum: 3, PartGroupKey: "manual-b", PartIndex: 1},
		{Base: model.Base{ID: "a-range"}, LibraryID: lib.ID, Title: "Show", Path: "/media/shows/Show/S02E03-E04.mkv", SeasonNum: 2, EpisodeNum: 3, EpisodeEndNum: 4, PartGroupKey: "manual-a", PartIndex: 5},
	}
	if err := e.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	showID := multipartSeriesID(lib.ID, "manual-a")
	root, err := e.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, card := range root["Items"].([]map[string]any) {
		if card["Id"] == showID && (card["RecursiveItemCount"] != 4 || card["ChildCount"] != 2) {
			t.Fatalf("SQL series card lost logical episode/season counts: %#v", card)
		}
	}
	result, err := e.Items(t.Context(), ItemsParams{ShowID: showID, ParentID: seasonID(showID, 2), IncludeItemTypes: []string{"Episode"}, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	items := result["Items"].([]map[string]any)
	if len(items) != 3 || result["TotalRecordCount"] != 3 {
		t.Fatalf("expected three logical season-2 episodes (including range): %#v", result)
	}
	for _, item := range items {
		if item["ParentIndexNumber"] != 2 || item["SeriesId"] != showID {
			t.Fatalf("lost season or series: %#v", item)
		}
		if item["Id"] == "a-1080" || item["Id"] == "a-2160" {
			sources := item["MediaSources"].([]map[string]any)
			if len(sources) != 2 {
				t.Fatalf("expected two isolated sources: %#v", sources)
			}
			for _, source := range sources {
				if source["Id"] != "a-1080" && source["Id"] != "a-2160" {
					t.Fatalf("leaked source: %#v", source)
				}
			}
		}
	}
	group, ok, err := e.findSeasonGroup(t.Context(), seasonID(showID, 0), "")
	if err != nil || !ok || len(group.Episodes) != 1 || group.Episodes[0].ID != "a-special" {
		t.Fatalf("specials lost: %#v %v %v", group, ok, err)
	}
	for _, id := range []string{"a-1080", "a-2160"} {
		item, err := e.Item(t.Context(), id, "")
		if err != nil {
			t.Fatal(err)
		}
		if item["ParentIndexNumber"] != 2 || item["IndexNumber"] != 3 || len(item["MediaSources"].([]map[string]any)) != 2 {
			t.Fatalf("direct version incorrect: %#v", item)
		}
	}
	counts, err := e.ItemCounts(t.Context(), "")
	if err != nil || counts["EpisodeCount"] != int64(5) || counts["SeriesCount"] != 2 {
		t.Fatalf("manual versions inflated counts: %#v %v", counts, err)
	}
}

func TestEmbyMatchedSeriesIdentityIgnoresEpisodeMetadata(t *testing.T) {
	e := newTestEmbyService(t)
	lib := model.Library{Name: "Shows", Path: "/shows", Type: "tv", Enabled: true}
	if err := e.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "episode-a"}, LibraryID: lib.ID, Title: "The Beginning", EpisodeTitle: "Opening", Overview: "one", PosterURL: "https://example.com/a.jpg", BackdropURL: "https://example.com/a-back.jpg", Path: "/shows/A/S01E01.mkv", SeasonNum: 1, EpisodeNum: 1, TMDbID: 123, ScrapeStatus: "matched"},
		{Base: model.Base{ID: "episode-b"}, LibraryID: lib.ID, Title: "The Ending", EpisodeTitle: "Finale", Overview: "two", PosterURL: "https://example.com/b.jpg", BackdropURL: "https://example.com/b-back.jpg", Path: "/shows/B/S01E02.mkv", SeasonNum: 1, EpisodeNum: 2, TMDbID: 123, ScrapeStatus: "matched"},
	}
	if err := e.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := e.repo.Media.RefreshEmbyKeys(t.Context(), e.repo.DB, []string{"episode-a", "episode-b"}); err != nil {
		t.Fatal(err)
	}
	var stored []model.Media
	if err := e.repo.DB.Order("id").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored[0].EmbySeriesKey != stored[1].EmbySeriesKey {
		t.Fatalf("persisted projection split same TMDb series: %s %s", stored[0].EmbySeriesKey, stored[1].EmbySeriesKey)
	}
	group, ok, err := e.findSeriesGroup(t.Context(), stored[0].EmbySeriesKey, "")
	if err != nil || !ok || len(group.Episodes) != 2 {
		t.Fatalf("split series: %#v %v %v", group, ok, err)
	}
	if group.Episodes[0].PosterURL == group.Episodes[1].PosterURL || group.Episodes[0].Overview == group.Episodes[1].Overview {
		t.Fatal("episode metadata was overwritten")
	}
	other := rows[1]
	other.TMDbID = 456
	if e.seriesIDForMedia(&other) == e.seriesIDForMedia(&rows[0]) {
		t.Fatal("different series merged")
	}
}
