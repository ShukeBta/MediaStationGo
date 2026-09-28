package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbySearchTermMatchesCaseInsensitiveTitleAndPath(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Adult", Path: `/media/adult`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "mide-949"}, LibraryID: lib.ID, Title: "MIDE-949", Path: `/media/adult/MIDE-949/MIDE-949.mp4`},
		{Base: model.Base{ID: "fc2"}, LibraryID: lib.ID, Title: "Unknown", Path: `/media/adult/FC2-PPV-926114/video.mp4`},
		{Base: model.Base{ID: "other"}, LibraryID: lib.ID, Title: "ABF-363", Path: `/media/adult/ABF-363/ABF-363.mp4`},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, SearchTerm: "mide-949", Limit: 50})
	if err != nil {
		t.Fatalf("items by title: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "mide-949" {
		t.Fatalf("SearchTerm should match title ignoring case, got %#v", items)
	}

	out, err = svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, SearchTerm: "mide 949", Limit: 50})
	if err != nil {
		t.Fatalf("items by split title terms: %v", err)
	}
	items = out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "mide-949" {
		t.Fatalf("SearchTerm should split title terms, got %#v", items)
	}

	out, err = svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, SearchTerm: "fc2-ppv-926114", Limit: 50})
	if err != nil {
		t.Fatalf("items by path: %v", err)
	}
	items = out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "fc2" {
		t.Fatalf("SearchTerm should match path ignoring case, got %#v", items)
	}

	out, err = svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, SearchTerm: "FC2 926114", Limit: 50})
	if err != nil {
		t.Fatalf("items by split path terms: %v", err)
	}
	items = out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "fc2" {
		t.Fatalf("SearchTerm should split path terms ignoring case, got %#v", items)
	}
}

func TestEmbySearchTermMatchesPinyinAndCompactCode(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "媒体", Path: `/media/all`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "men-in-black"}, LibraryID: lib.ID, Title: "黑衣人", Path: `/media/all/黑衣人.mkv`, Genres: "科幻"},
		{Base: model.Base{ID: "mizd-534"}, LibraryID: lib.ID, Title: "MIZD-534", Path: `/media/all/MIZD-534.mp4`},
	}
	for i := range rows {
		if err := svc.repo.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	for query, wantID := range map[string]string{
		"heiyiren": "men-in-black",
		"hyr":      "men-in-black",
		"kehuan":   "men-in-black",
		"mizd534":  "mizd-534",
	} {
		t.Run(query, func(t *testing.T) {
			out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, SearchTerm: query, Limit: 50})
			if err != nil {
				t.Fatalf("items: %v", err)
			}
			items := out["Items"].([]map[string]any)
			if len(items) != 1 || items[0]["Id"] != wantID {
				t.Fatalf("SearchTerm %q = %#v, want %q", query, items, wantID)
			}
		})
	}
}

func TestEmbyLibraryViewExposesExplicitAdultType(t *testing.T) {
	svc := newTestEmbyService(t)
	view := svc.libraryAsView(t.Context(), &model.Library{
		Base:    model.Base{ID: "adult-library"},
		Name:    "成人",
		Path:    "/media/adult",
		Type:    "adult",
		Enabled: true,
	})
	if view["CollectionType"] != "movies" || view["MediaStationLibraryType"] != "adult" {
		t.Fatalf("adult library view = %#v", view)
	}
}

func TestEmbyNameStartsWithMatchesTitlePrefixOnly(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "Adult", Path: `/media/adult`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "mide-949"}, LibraryID: lib.ID, Title: "MIDE-949", Path: `/media/adult/MIDE-949/MIDE-949.mp4`},
		{Base: model.Base{ID: "amide"}, LibraryID: lib.ID, Title: "AMIDE", Path: `/media/adult/AMIDE/AMIDE.mp4`},
		{Base: model.Base{ID: "path-only"}, LibraryID: lib.ID, Title: "Unknown", Path: `/media/adult/MID-PATH/video.mp4`},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, NameStartsWith: "mid", Limit: 50})
	if err != nil {
		t.Fatalf("items by prefix: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "mide-949" {
		t.Fatalf("NameStartsWith should match title prefix ignoring case, got %#v", items)
	}

	out, err = svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, NameStartsWith: "ide", Limit: 50})
	if err != nil {
		t.Fatalf("items by non-prefix: %v", err)
	}
	items = out["Items"].([]map[string]any)
	if len(items) != 0 {
		t.Fatalf("NameStartsWith should not behave like contains search, got %#v", items)
	}
}

func TestEmbyGlobalSearchIgnoresClientItemTypeHints(t *testing.T) {
	svc := newTestEmbyService(t)
	movieLib := model.Library{Name: "电影", Path: `/media/movie`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &movieLib); err != nil {
		t.Fatalf("create movie library: %v", err)
	}
	tvLib := model.Library{Name: "剧集", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &tvLib); err != nil {
		t.Fatalf("create tv library: %v", err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "mermaid-movie"}, LibraryID: movieLib.ID, Title: "美人鱼", Path: `/media/movie/美人鱼.mkv`},
		{Base: model.Base{ID: "mermaid-series-ep1"}, LibraryID: tvLib.ID, Title: "美人鱼剧集", Path: `/media/tv/美人鱼剧集/Season 1/E01.mkv`, SeasonNum: 1, EpisodeNum: 1},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		UserID:           "user-1",
		SearchTerm:       "美人鱼",
		IncludeItemTypes: []string{"Movie", "Series"},
		Recursive:        true,
		Limit:            20,
	})
	if err != nil {
		t.Fatalf("items by global search with client item type hints: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("global search should ignore client Movie,Series hints and include matching media rows, got %#v", items)
	}
	typesByID := map[string]any{}
	for _, item := range items {
		if id, ok := item["Id"].(string); ok {
			typesByID[id] = item["Type"]
		}
	}
	if typesByID["mermaid-movie"] != "Movie" {
		t.Fatalf("global search with client hints dropped Movie result: %#v", items)
	}
	if typesByID["mermaid-series-ep1"] != "Episode" {
		t.Fatalf("global search with client hints dropped episodic result: %#v", items)
	}
}

func TestNormalizeEmbyGlobalSearchParamsIgnoresClientItemTypeHints(t *testing.T) {
	cases := []struct {
		name string
		in   ItemsParams
		want []string
	}{
		{name: "default search", in: ItemsParams{SearchTerm: "美人鱼"}, want: nil},
		{name: "movie only global search", in: ItemsParams{SearchTerm: "美人鱼", IncludeItemTypes: []string{"Movie"}}, want: nil},
		{name: "series only global search", in: ItemsParams{SearchTerm: "美人鱼", IncludeItemTypes: []string{"Series"}}, want: nil},
		{name: "movie and series global search", in: ItemsParams{SearchTerm: "美人鱼", IncludeItemTypes: []string{"Movie", "Series"}}, want: nil},
		{name: "movie and series without search", in: ItemsParams{IncludeItemTypes: []string{"Movie", "Series"}}, want: []string{"Movie", "Series"}},
		{name: "library scoped search keeps hints", in: ItemsParams{ParentID: "library-1", SearchTerm: "美人鱼", IncludeItemTypes: []string{"Movie", "Series"}}, want: []string{"Movie", "Series"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeEmbyGlobalSearchParams(tc.in).IncludeItemTypes
			if len(got) != len(tc.want) {
				t.Fatalf("IncludeItemTypes len = %d, want %d: %#v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("IncludeItemTypes = %#v, want %#v", got, tc.want)
				}
			}
		})
	}
}
