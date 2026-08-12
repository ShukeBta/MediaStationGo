package service

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const embyDateCreatedPageSize = 48

func TestEmbyMediaItemsDateCreatedPagesUsePublicIDTieBreak(t *testing.T) {
	svc := newTestEmbyService(t)
	library := model.Library{Name: "Movies", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &library); err != nil {
		t.Fatalf("create library: %v", err)
	}
	createdAt := time.Date(2026, time.August, 12, 1, 2, 3, 0, time.UTC)
	rows := make([]model.Media, 0, 97)
	for index := 0; index < 97; index++ {
		rows = append(rows, model.Media{
			Base:      model.Base{ID: fmt.Sprintf("media-%03d", index), CreatedAt: createdAt},
			LibraryID: library.ID,
			Title:     fmt.Sprintf("Movie %03d", index),
			TMDbID:    10000 + index,
			Path:      fmt.Sprintf("/media/movies/media-%03d.mkv", index),
		})
	}
	if err := svc.repo.DB.Create(&rows).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	assertEmbyDateCreatedPages(t, svc, ItemsParams{
		ParentID:         library.ID,
		Recursive:        true,
		IncludeItemTypes: []string{"Movie"},
		SortBy:           "DateCreated",
		SortOrder:        "Descending",
	}, 97)
}

func TestEmbyMovieLibraryDateCreatedPagesUsePublicIDTieBreak(t *testing.T) {
	svc := newTestEmbyService(t)
	library := model.Library{Name: "Mixed Movies", Path: "/media/mixed", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &library); err != nil {
		t.Fatalf("create library: %v", err)
	}
	createdAt := time.Date(2026, time.August, 12, 1, 2, 3, 0, time.UTC)
	rows := []model.Media{
		{Base: model.Base{ID: "bundle-part-1", CreatedAt: createdAt}, LibraryID: library.ID, Title: "Virtual Bundle", Path: "/media/mixed/bundle/part-1.mkv", PartGroupKey: "bundle", PartGroupTitle: "Virtual Bundle", PartIndex: 1},
		{Base: model.Base{ID: "bundle-part-2", CreatedAt: createdAt}, LibraryID: library.ID, Title: "Virtual Bundle", Path: "/media/mixed/bundle/part-2.mkv", PartGroupKey: "bundle", PartGroupTitle: "Virtual Bundle", PartIndex: 2},
	}
	for index := 0; index < 96; index++ {
		rows = append(rows, model.Media{
			Base:      model.Base{ID: fmt.Sprintf("movie-%03d", index), CreatedAt: createdAt},
			LibraryID: library.ID,
			Title:     fmt.Sprintf("Movie %03d", index),
			TMDbID:    20000 + index,
			Path:      fmt.Sprintf("/media/mixed/movie-%03d.mkv", index),
		})
	}
	if err := svc.repo.DB.Create(&rows).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	assertEmbyDateCreatedPages(t, svc, ItemsParams{
		ParentID:         library.ID,
		Recursive:        true,
		IncludeItemTypes: []string{"Movie"},
		SortBy:           "DateCreated",
		SortOrder:        "Descending",
	}, 97)
}

func TestEmbySeriesItemsDateCreatedPagesUsePublicIDTieBreak(t *testing.T) {
	svc := newTestEmbyService(t)
	library := model.Library{Name: "Series", Path: "/media/series", Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &library); err != nil {
		t.Fatalf("create library: %v", err)
	}
	createdAt := time.Date(2026, time.August, 12, 1, 2, 3, 0, time.UTC)
	rows := make([]model.Media, 0, 97)
	for index := 0; index < 97; index++ {
		rows = append(rows, model.Media{
			Base:       model.Base{ID: fmt.Sprintf("episode-%03d", 96-index), CreatedAt: createdAt},
			LibraryID:  library.ID,
			SeriesID:   fmt.Sprintf("series-%03d", index),
			Title:      fmt.Sprintf("Series %03d", index),
			Path:       fmt.Sprintf("/media/series/Series %03d/Season 01/S01E01.mkv", index),
			SeasonNum:  1,
			EpisodeNum: 1,
		})
	}
	if err := svc.repo.DB.Create(&rows).Error; err != nil {
		t.Fatalf("create episodes: %v", err)
	}

	assertEmbyDateCreatedPages(t, svc, ItemsParams{
		ParentID:         library.ID,
		Recursive:        true,
		IncludeItemTypes: []string{"Series"},
		SortBy:           "DateCreated",
		SortOrder:        "Descending",
	}, 97)
}

func assertEmbyDateCreatedPages(t *testing.T, svc *EmbyService, params ItemsParams, wantTotal int) {
	t.Helper()
	page := func(startIndex int) []string {
		t.Helper()
		request := params
		request.StartIndex = startIndex
		request.Limit = embyDateCreatedPageSize
		out, err := svc.Items(t.Context(), request)
		if err != nil {
			t.Fatalf("items start=%d: %v", startIndex, err)
		}
		if got := embyTotalRecordCount(t, out); got != wantTotal {
			t.Fatalf("items start=%d total=%d, want %d", startIndex, got, wantTotal)
		}
		return embyItemIDs(t, out)
	}

	first := page(0)
	second := page(embyDateCreatedPageSize)
	third := page(embyDateCreatedPageSize * 2)
	if len(first) != embyDateCreatedPageSize || len(second) != embyDateCreatedPageSize || len(third) != 1 {
		t.Fatalf("page sizes = %d/%d/%d, want 48/48/1", len(first), len(second), len(third))
	}
	if again := page(0); !reflect.DeepEqual(again, first) {
		t.Fatalf("first page is not repeatable: first=%v again=%v", first, again)
	}
	if again := page(embyDateCreatedPageSize); !reflect.DeepEqual(again, second) {
		t.Fatalf("second page is not repeatable: first=%v again=%v", second, again)
	}

	all := append(append(append([]string{}, first...), second...), third...)
	if len(all) != wantTotal {
		t.Fatalf("paged Id count=%d, want %d", len(all), wantTotal)
	}
	seen := make(map[string]struct{}, len(all))
	for _, id := range all {
		if _, duplicate := seen[id]; duplicate {
			t.Fatalf("DateCreated pages overlap on public Id %q: first=%v second=%v", id, first, second)
		}
		seen[id] = struct{}{}
	}
	wantOrder := append([]string(nil), all...)
	sort.Slice(wantOrder, func(i, j int) bool { return wantOrder[i] > wantOrder[j] })
	if !reflect.DeepEqual(all, wantOrder) {
		t.Fatalf("DateCreated pages are not ordered by public Id descending: got=%v want=%v", all, wantOrder)
	}
}

func embyItemIDs(t *testing.T, out map[string]any) []string {
	t.Helper()
	items, ok := out["Items"].([]map[string]any)
	if !ok {
		t.Fatalf("Items type=%T, want []map[string]any", out["Items"])
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		id, ok := item["Id"].(string)
		if !ok || id == "" {
			t.Fatalf("item missing public Id: %#v", item)
		}
		ids = append(ids, id)
	}
	return ids
}

func embyTotalRecordCount(t *testing.T, out map[string]any) int {
	t.Helper()
	switch value := out["TotalRecordCount"].(type) {
	case int:
		return value
	case int64:
		return int(value)
	default:
		t.Fatalf("TotalRecordCount type=%T, want int or int64", value)
		return 0
	}
}
