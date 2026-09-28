package service

import (
	"reflect"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbySeriesSQLClientSortAndMissingDates(t *testing.T) {
	e, lib := embyProjectionFixture(t, 0, 0)
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := []model.Media{
		{SeriesID: "a", Title: "Zulu", ReleaseDate: "2020-01-01", Year: 2020},
		{SeriesID: "b", Title: "Alpha", ReleaseDate: "2021-01-01", Year: 2021},
		{SeriesID: "c", Title: "Missing Z", Year: 9999},
		{SeriesID: "d", Title: "Missing A", ReleaseDate: "invalid", Year: 2000},
		{SeriesID: "e", Title: "Alpha", ReleaseDate: "2021-01-01", Year: 2021},
		{SeriesID: "f", Title: "Bravo", ReleaseDate: "2021-01-01", Year: 2021},
	}
	for i := range rows {
		rows[i].ID = "episode-" + rows[i].SeriesID
		rows[i].LibraryID, rows[i].SeasonNum, rows[i].EpisodeNum = lib.ID, 1, 1
		rows[i].Path = "/media/" + rows[i].ID + ".mkv"
		rows[i].CreatedAt, rows[i].UpdatedAt = stamp, stamp
		e.repo.Media.PrepareEmbyKeys(&rows[i])
	}
	if err := e.repo.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		sort, direction string
		want            []string
	}{
		{"PremiereDate", "Descending", []string{"b", "e", "f", "a", "d", "c"}},
		{"PremiereDate", "Ascending", []string{"a", "b", "e", "f", "d", "c"}},
		{"PremiereDate,SortName", "Descending,Descending", []string{"f", "b", "e", "a", "d", "c"}},
		{"DateLastContentAdded,SortName", "Descending,Descending", []string{"a", "c", "d", "f", "b", "e"}},
		{"SortName", "Ascending", []string{"b", "e", "f", "d", "c", "a"}},
	}
	for _, tc := range cases {
		p := ItemsParams{Limit: 2, SortBy: tc.sort, SortOrder: tc.direction}
		var got []string
		for offset := 0; offset < len(rows); offset += 2 {
			p.StartIndex = offset
			page, err := e.seriesItemsForLibrary(t.Context(), lib.ID, p)
			if err != nil {
				t.Fatal(err)
			}
			if page["TotalRecordCount"] != len(rows) {
				t.Fatal("incorrect total")
			}
			for _, item := range page["Items"].([]map[string]any) {
				got = append(got, item["Id"].(string))
			}
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("sort=%s order=%s: got %v want %v", tc.sort, tc.direction, got, tc.want)
		}
		groups, err := e.seriesGroupsFromMedia(t.Context(), rows)
		if err != nil {
			t.Fatal(err)
		}
		sortSeriesGroups(groups, p)
		ids := make([]string, len(groups))
		for i := range groups {
			ids[i] = groups[i].ID
		}
		if !reflect.DeepEqual(ids, tc.want) {
			t.Fatalf("Go/SQL sorting diverged: %v vs %v", ids, tc.want)
		}
	}
}

func TestEmbySeriesSortTransitiveWithMissingFields(t *testing.T) {
	groups := []embySeriesGroup{
		{ID: "a", Name: "Z", ReleaseDate: "2025-01-01"},
		{ID: "b", Name: "B"},
		{ID: "c", Name: "A", ReleaseDate: "2024-01-01"},
		{ID: "d", Name: "B", ReleaseDate: "invalid"},
	}
	terms := embySeriesSortTerms(ItemsParams{SortBy: "PremiereDate,SortName", SortOrder: "Descending,Ascending"})
	for _, a := range groups {
		for _, b := range groups {
			for _, c := range groups {
				if compareEmbySeries(a, b, terms) < 0 && compareEmbySeries(b, c, terms) < 0 && compareEmbySeries(a, c, terms) >= 0 {
					t.Fatal("non-transitive ordering")
				}
			}
		}
	}
}
