package service

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func newBrowseTestService(t *testing.T, kind string) (*MediaService, *repository.Container, model.Library) {
	t.Helper()
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	lib := model.Library{Name: "Browse", Path: "/media/" + kind, Type: kind, Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	return NewMediaService(&config.Config{}, zap.NewNop(), repos), repos, lib
}

func browseFixture(lib model.Library, i int) model.Media {
	stamp := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(i) * time.Minute)
	return model.Media{
		Base:      model.Base{ID: fmt.Sprintf("browse-%03d", i), CreatedAt: stamp, UpdatedAt: stamp},
		LibraryID: lib.ID, Title: fmt.Sprintf("Work %03d", i),
		Path:   fmt.Sprintf("%s/Work %03d/file.mkv", lib.Path, i),
		TMDbID: 10000 + i, Countries: "US", Languages: "en", ScrapeStatus: "matched",
	}
}

func TestBrowseLibrarySeriesPaginationFacetsAndDirectLink(t *testing.T) {
	svc, repos, lib := newBrowseTestService(t, "tv")
	visibility := MediaVisibility{IncludeNSFW: true}
	for i := 0; i < 177; i++ {
		m := browseFixture(lib, i)
		m.SeasonNum, m.EpisodeNum = 1, 1
		// The identity sample and real poster deliberately disagree on category.
		m.Countries, m.Languages = "JP", "ja"
		if err := repos.Media.Upsert(t.Context(), &m); err != nil {
			t.Fatal(err)
		}
		m.ID += "-poster"
		m.Path = fmt.Sprintf("%s/Work %03d/episode2.mkv", lib.Path, i)
		m.EpisodeNum, m.PosterURL = 2, "https://image.example/poster.jpg"
		if i >= 60 {
			m.Countries, m.Languages = "US", "en"
		}
		if err := repos.Media.Upsert(t.Context(), &m); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true}, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 177 || len(first.SeriesCards) != 48 || first.Page != 1 || first.PageSize != 48 || !first.IsSeries {
		t.Fatalf("first page total=%d cards=%d page=%d", first.Total, len(first.SeriesCards), first.Page)
	}
	if !reflect.DeepEqual(first.Facets.Categories, []LibraryBrowseFacet{{Name: "日韩剧", Count: 60}, {Name: "欧美剧", Count: 117}}) {
		t.Fatalf("facets must count representative works, not identity samples/episodes: %#v", first.Facets)
	}
	all, _, err := svc.ListLibrarySeriesCards(t.Context(), lib.ID, 1, 500, visibility)
	if err != nil {
		t.Fatal(err)
	}
	for page := 1; page <= 4; page++ {
		got, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: page}, visibility)
		if err != nil {
			t.Fatal(err)
		}
		if got.Facets != nil || !reflect.DeepEqual(got.SeriesCards, all[(page-1)*48:min(page*48, 177)]) {
			t.Fatalf("page %d changed established order/artwork/payload", page)
		}
	}
	lastKey := all[176].Key
	filtered, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 2, Category: "日韩剧", SeriesKey: all[0].Key}, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != 60 || len(filtered.SeriesCards) != 12 || filtered.SelectedSeries == nil || filtered.SelectedSeries.Key != all[0].Key {
		t.Fatalf("cross-page filter/direct link: total=%d size=%d selected=%v", filtered.Total, len(filtered.SeriesCards), filtered.SelectedSeries)
	}
	for _, c := range filtered.SeriesCards {
		if c.Rep.AutoCategory != "日韩剧" || c.Count != 2 {
			t.Fatalf("incorrect filtered card: %#v", c)
		}
	}
	linked, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, SeriesKey: lastKey}, visibility)
	if err != nil || linked.SelectedSeries == nil || linked.SelectedSeries.Key != lastKey {
		t.Fatalf("off-page deep link failed: %v", err)
	}
	focused, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, FocusMediaID: "browse-000"}, visibility)
	if err != nil || focused.Page != 4 || focused.FocusedMediaID != "browse-000-poster" {
		t.Fatalf("focus non-representative episode page=%d err=%v", focused.Page, err)
	}
	empty, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 99, Category: "missing"}, visibility)
	if err != nil || empty.Page != 1 || empty.Total != 0 || len(empty.SeriesCards) != 0 {
		t.Fatalf("empty filter page: %#v err=%v", empty, err)
	}
	for i := 0; i < 33; i++ {
		if err := repos.DB.Where("tm_db_id = ?", 10000+i).Delete(&model.Media{}).Error; err != nil {
			t.Fatal(err)
		}
	}
	clamped, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 4}, visibility)
	if err != nil || clamped.Page != 3 || clamped.Total != 144 || len(clamped.SeriesCards) != 48 {
		t.Fatalf("deleted last page: total=%d page=%d err=%v", clamped.Total, clamped.Page, err)
	}
}

func TestBrowseLibraryMoviesGlobalActorFiltersAndIngest(t *testing.T) {
	svc, repos, lib := newBrowseTestService(t, "adult")
	visibility := MediaVisibility{IncludeNSFW: true}
	for i := 0; i < 97; i++ {
		m := browseFixture(lib, i)
		m.NSFW, m.Actors = true, "First, FIRST"
		if i < 49 {
			m.Actors = "Second, second"
			m.OriginalName = fmt.Sprintf("FC2-PPV-%07d", 1000000+i)
		}
		if err := repos.Media.Upsert(t.Context(), &m); err != nil {
			t.Fatal(err)
		}
	}
	first, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true}, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 97 || len(first.Items) != 48 || len(first.Facets.Actors) != 2 || first.IsSeries {
		t.Fatalf("movie first page: %#v", first)
	}
	counts := map[string]int{}
	for _, f := range first.Facets.AdultTypes {
		counts[f.Name] = f.Count
	}
	if counts["AV"] != 48 || counts["FC2"] != 49 {
		t.Fatalf("global type counts: %#v", counts)
	}
	all, _, err := svc.ListMediaVisibleGrouped(t.Context(), lib.ID, 1, 500, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first.Items, all[:48]) {
		t.Fatal("metadata path changed representative payload or order")
	}
	filtered, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 2, Actor: "second", AdultType: "FC2"}, visibility)
	if err != nil || filtered.Total != 49 || len(filtered.Items) != 1 || filtered.Items[0].Actors != "Second, second" {
		t.Fatalf("global actor filter total=%d size=%d err=%v", filtered.Total, len(filtered.Items), err)
	}
	if got, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true}, MediaVisibility{}); err == nil && (got.Total != 0 || len(got.Items) != 0) {
		t.Fatal("NSFW leaked")
	}
	newMedia := browseFixture(lib, 100)
	newMedia.NSFW, newMedia.Actors = true, "Third"
	if err := repos.Media.Upsert(t.Context(), &newMedia); err != nil {
		t.Fatal(err)
	}
	added, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true}, visibility)
	if err != nil || added.Total != 98 || added.Items[0].ID != newMedia.ID || len(added.Facets.Actors) != 3 {
		t.Fatalf("ingest not immediately visible: total=%d err=%v", added.Total, err)
	}
	other := model.Library{Name: "Other", Path: "/other", Type: "adult", Enabled: true}
	if err := repos.Library.Create(t.Context(), &other); err != nil {
		t.Fatal(err)
	}
	newMedia.LibraryID, newMedia.Path = other.ID, "/other/new.mkv"
	if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), repos.DB, newMedia.ID, map[string]any{"library_id": newMedia.LibraryID, "path": newMedia.Path}); err != nil {
		t.Fatal(err)
	}
	moved, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, Actor: "Third", IncludeFacets: true}, visibility)
	if err != nil || moved.Total != 0 || len(moved.Facets.Actors) != 2 {
		t.Fatalf("moved work remained in source: %#v err=%v", moved, err)
	}
	target, err := svc.BrowseLibrary(t.Context(), other.ID, LibraryBrowseOptions{Page: 1}, visibility)
	if err != nil || target.Total != 1 || target.Items[0].ID != newMedia.ID {
		t.Fatalf("moved work missing from target: %v", err)
	}
}

func TestBrowseLibraryVersionAndPartFacetsMatchRepresentatives(t *testing.T) {
	svc, repos, lib := newBrowseTestService(t, "movie")
	low, high := browseFixture(lib, 0), browseFixture(lib, 1)
	high.TMDbID, high.Title = low.TMDbID, low.Title
	low.Countries, high.Countries = "JP", "US"
	low.Width, low.Height, high.Width, high.Height = 1280, 720, 3840, 2160
	for _, m := range []*model.Media{&low, &high} {
		if err := repos.Media.Upsert(t.Context(), m); err != nil {
			t.Fatal(err)
		}
	}
	got, err := svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true}, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Items[0].ID != high.ID || len(got.Items[0].Versions) != 2 || !reflect.DeepEqual(got.Facets.Categories, []LibraryBrowseFacet{{Name: "欧美电影", Count: 1}}) {
		t.Fatalf("version representative mismatch: %#v", got)
	}
	if _, err := svc.UpdateMediaAggregation(t.Context(), lib.ID, MediaAggregationRequest{Action: MediaAggregationActionGroup, Title: "Grouped work", MediaIDs: []string{low.ID, high.ID}}); err != nil {
		t.Fatal(err)
	}
	got, err = svc.BrowseLibrary(t.Context(), lib.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true}, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Items[0].ID != low.ID || len(got.Items[0].Parts) != 2 || !reflect.DeepEqual(got.Facets.Categories, []LibraryBrowseFacet{{Name: "日韩电影", Count: 1}}) {
		t.Fatalf("part representative mismatch: %#v", got)
	}
}

func TestBrowseLibraryMergedSeriesVisibility(t *testing.T) {
	svc, repos, _ := newBrowseTestService(t, "tv")
	root := model.Library{Name: "剧集", Path: "cloud://openlist/115/剧集", Type: "tv", Enabled: true}
	child := model.Library{Name: "剧集", Path: "cloud://openlist/115/剧集/国产剧", Type: "tv", Enabled: true}
	for _, lib := range []*model.Library{&root, &child} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	a := model.Media{Base: model.Base{ID: "merged-a"}, LibraryID: root.ID, Title: "云端剧", Path: "cloud://openlist/115/剧集/云端剧/Season 1/E01.mkv", SeasonNum: 1, EpisodeNum: 1}
	b := model.Media{Base: model.Base{ID: "merged-b"}, LibraryID: child.ID, Title: "云端剧", Path: "cloud://openlist/115/剧集/国产剧/云端剧/Season 1/E02.mkv", SeasonNum: 1, EpisodeNum: 2}
	for _, m := range []*model.Media{&a, &b} {
		if err := repos.Media.Upsert(t.Context(), m); err != nil {
			t.Fatal(err)
		}
	}
	all, err := svc.BrowseLibrary(t.Context(), root.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true}, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if all.Total != 1 || all.SeriesCards[0].Count != 2 || len(all.Facets.Categories) != 1 || all.Facets.Categories[0].Count != 1 {
		t.Fatalf("merged facets count episodes instead of works: %#v", all)
	}
	visible, err := svc.BrowseLibrary(t.Context(), root.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true, SeriesKey: all.SeriesCards[0].Key, FocusMediaID: b.ID}, MediaVisibility{IncludeNSFW: true, HiddenLibraryIDs: []string{child.ID}})
	if err != nil {
		t.Fatal(err)
	}
	// Hidden physical libraries expand to their logical merged scope; neither
	// cards nor facets nor independently resolved detail may bypass that rule.
	if visible.Total != 0 || len(visible.SeriesCards) != 0 || visible.SelectedSeries != nil || len(visible.Facets.Categories) != 0 {
		t.Fatalf("hidden library leaked through facet/detail/focus: %#v", visible)
	}
	if _, err := svc.BrowseLibrary(t.Context(), root.ID, LibraryBrowseOptions{Page: 1, IncludeFacets: true}, MediaVisibility{IncludeNSFW: true, AllowedLibraryIDs: []string{"not-allowed"}}); err == nil {
		t.Fatal("inaccessible library accepted")
	}
}
