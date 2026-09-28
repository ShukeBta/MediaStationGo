package service

import (
	"slices"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyGenresExposeSmartCategoriesAndFilterItems(t *testing.T) {
	svc := newTestEmbyService(t)
	movieLibrary := model.Library{Name: "电影", Path: "/media/电影", Type: "movie", Enabled: true}
	adultLibrary := model.Library{Name: "成人", Path: "/media/成人", Type: "adult", Enabled: true}
	for _, library := range []*model.Library{&movieLibrary, &adultLibrary} {
		if err := svc.repo.Library.Create(t.Context(), library); err != nil {
			t.Fatal(err)
		}
	}
	rows := []model.Media{
		{Base: model.Base{ID: "cn-movie"}, LibraryID: movieLibrary.ID, Title: "流浪地球", Path: "/media/电影/流浪地球.mkv", Countries: "CN", Genres: "科幻,冒险"},
		{Base: model.Base{ID: "us-movie"}, LibraryID: movieLibrary.ID, Title: "Dune", Path: "/media/电影/Dune.mkv", Countries: "US", Genres: "科幻,冒险"},
		{Base: model.Base{ID: "adult-movie"}, LibraryID: adultLibrary.ID, Title: "ABC-123", Path: "/media/成人/ABC-123.mp4", Genres: "Adult,javdb,无码", NSFW: true},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	genres, err := svc.Genres(t.Context(), ItemsParams{ParentID: movieLibrary.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	names := genrePayloadNames(genres["Items"].([]map[string]any))
	for _, want := range []string{"华语电影", "欧美电影", "科幻", "冒险"} {
		if !slices.Contains(names, want) {
			t.Fatalf("genres = %#v, missing %q", names, want)
		}
	}

	items, err := svc.Items(t.Context(), ItemsParams{
		ParentID: movieLibrary.ID,
		GenreIDs: []string{embyGenreID("华语电影")},
		Limit:    50,
	})
	if err != nil {
		t.Fatal(err)
	}
	filtered := items["Items"].([]map[string]any)
	if len(filtered) != 1 || filtered[0]["Id"] != "cn-movie" {
		t.Fatalf("smart category filter = %#v", filtered)
	}
	genreItems, ok := filtered[0]["GenreItems"].([]map[string]string)
	if !ok || len(genreItems) == 0 {
		t.Fatalf("item genre ids = %#v", filtered[0]["GenreItems"])
	}

	byFolder, err := svc.Items(t.Context(), ItemsParams{ParentID: embyGenreID("欧美电影"), Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	folderItems := byFolder["Items"].([]map[string]any)
	if len(folderItems) != 1 || folderItems[0]["Id"] != "us-movie" {
		t.Fatalf("genre folder filter = %#v", folderItems)
	}
}

func TestEmbyGenresRespectAdultVisibility(t *testing.T) {
	svc := newTestEmbyService(t)
	movieLibrary := model.Library{Name: "电影", Path: "/media/电影", Type: "movie", Enabled: true}
	adultLibrary := model.Library{Name: "成人", Path: "/media/成人", Type: "adult", Enabled: true}
	for _, library := range []*model.Library{&movieLibrary, &adultLibrary} {
		if err := svc.repo.Library.Create(t.Context(), library); err != nil {
			t.Fatal(err)
		}
	}
	viewer := model.User{
		Base:                model.Base{ID: "viewer"},
		Username:            "viewer",
		PasswordHash:        "x",
		Role:                "user",
		AllowedLibraryIDs:   []string{movieLibrary.ID, adultLibrary.ID},
		HideAdult:           false,
		AdultContentBlocked: true,
		IsActive:            true,
	}
	if err := svc.repo.User.Create(t.Context(), &viewer); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "regular"}, LibraryID: movieLibrary.ID, Title: "普通电影", Path: "/media/电影/regular.mkv", Countries: "CN", Genres: "剧情"},
		{Base: model.Base{ID: "adult"}, LibraryID: adultLibrary.ID, Title: "ABC-123", Path: "/media/成人/ABC-123.mp4", Genres: "Adult,javdb,无码", NSFW: true},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	genres, err := svc.Genres(t.Context(), ItemsParams{UserID: viewer.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	names := genrePayloadNames(genres["Items"].([]map[string]any))
	for _, hidden := range []string{"成人", "无码", "Adult", "javdb"} {
		if slices.Contains(names, hidden) {
			t.Fatalf("restricted genres leaked %q in %#v", hidden, names)
		}
	}
	if !slices.Contains(names, "华语电影") || !slices.Contains(names, "剧情") {
		t.Fatalf("visible genres missing from %#v", names)
	}
}

func TestEmbyAdultGenresSeparateAVAndFC2(t *testing.T) {
	svc := newTestEmbyService(t)
	adultLibrary := model.Library{Name: "成人", Path: "/media/成人", Type: "adult", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &adultLibrary); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{Base: model.Base{ID: "av"}, LibraryID: adultLibrary.ID, Title: "MIZD-534", OriginalName: "MIZD-534", Path: "/media/成人/MIZD-534.mp4", Genres: "Adult,javdb", NSFW: true},
		{Base: model.Base{ID: "fc2"}, LibraryID: adultLibrary.ID, Title: "FC2 作品", OriginalName: "FC2-PPV-3780016", Path: "/media/成人/FC2-PPV-3780016.mp4", Genres: "Adult,fd2ppv", NSFW: true},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	genres, err := svc.Genres(t.Context(), ItemsParams{ParentID: adultLibrary.ID, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	names := genrePayloadNames(genres["Items"].([]map[string]any))
	for _, want := range []string{adultMediaTypeAV, adultMediaTypeFC2} {
		if !slices.Contains(names, want) {
			t.Fatalf("genres = %#v, missing %q", names, want)
		}
	}
	if slices.Contains(names, "成人") {
		t.Fatalf("adult smart category should be replaced by AV/FC2, got %#v", names)
	}

	items, err := svc.Items(t.Context(), ItemsParams{
		ParentID: adultLibrary.ID,
		GenreIDs: []string{embyGenreID(adultMediaTypeFC2)},
		Limit:    50,
	})
	if err != nil {
		t.Fatal(err)
	}
	filtered := items["Items"].([]map[string]any)
	if len(filtered) != 1 || filtered[0]["Id"] != "fc2" {
		t.Fatalf("FC2 genre filter = %#v", filtered)
	}
}

func genrePayloadNames(items []map[string]any) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		if name, ok := item["Name"].(string); ok {
			names = append(names, name)
		}
	}
	return names
}
