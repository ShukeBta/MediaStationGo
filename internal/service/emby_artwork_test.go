package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestImageURLFallsBackToLibraryCoverURL(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true, CoverURL: "https://example.com/lib-cover.jpg"}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}

	raw, err := svc.ImageURL(t.Context(), lib.ID, "Primary")
	if err != nil {
		t.Fatalf("ImageURL: %v", err)
	}
	if raw != lib.CoverURL {
		t.Fatalf("ImageURL for library = %q, want cover_url %q", raw, lib.CoverURL)
	}
}

func TestImageURLFallsBackToBestMemberPoster(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true} // no cover_url
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	members := []model.Media{
		{LibraryID: lib.ID, Title: "A", Path: "/media/movies/a.mkv", PosterURL: "https://example.com/a/backdrop.jpg"},
		{LibraryID: lib.ID, Title: "B", Path: "/media/movies/b.mkv", PosterURL: "https://example.com/b/poster.jpg"},
		{LibraryID: lib.ID, Title: "C", Path: "/media/movies/c.mkv", PosterURL: "https://example.com/c/actor.jpg"},
	}
	for i := range members {
		if err := svc.repo.DB.Create(&members[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	raw, err := svc.ImageURL(t.Context(), lib.ID, "Primary")
	if err != nil {
		t.Fatalf("ImageURL: %v", err)
	}
	// 最高分是 poster.jpg(score 40),其次是默认海报(30),actor(10)。
	if raw != "https://example.com/b/poster.jpg" {
		t.Fatalf("ImageURL for library = %q, want best member poster", raw)
	}
}

func TestImageURLReturnsEmptyForUnknownID(t *testing.T) {
	svc := newTestEmbyService(t)
	raw, err := svc.ImageURL(t.Context(), "not-a-library-or-media", "Primary")
	if err != nil {
		t.Fatalf("ImageURL: %v", err)
	}
	if raw != "" {
		t.Fatalf("ImageURL for unknown id = %q, want empty", raw)
	}
}

func TestViewsAdvertisesPrimaryImageWhenLibraryHasCover(t *testing.T) {
	svc := newTestEmbyService(t)
	withCover := model.Library{Name: "封库", Path: "/media/set", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &withCover); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{
		Base:      model.Base{ID: "m1"},
		LibraryID: withCover.ID,
		Title:     "M1",
		Path:      "/media/set/m1.mkv",
		PosterURL: "https://example.com/m1/poster.jpg",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	noCover := model.Library{Name: "空库", Path: "/media/empty", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &noCover); err != nil {
		t.Fatalf("create library: %v", err)
	}

	views, err := svc.Views(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("Views: %v", err)
	}
	items := views["Items"].([]map[string]any)
	found := map[string]map[string]any{}
	for _, it := range items {
		found[it["Id"].(string)] = it
	}

	withImageTags, ok := found[withCover.ID]["ImageTags"].(map[string]string)
	if !ok {
		t.Fatalf("library with cover should have ImageTags, got %#v", found[withCover.ID]["ImageTags"])
	}
	if withImageTags["Primary"] != withCover.ID {
		t.Fatalf("library with cover should advertise ImageTags.Primary, got %#v", withImageTags)
	}

	emptyTags, ok := found[noCover.ID]["ImageTags"].(map[string]string)
	if !ok {
		t.Fatalf("library without cover should have empty ImageTags, got %#v", found[noCover.ID]["ImageTags"])
	}
	if _, has := emptyTags["Primary"]; has {
		t.Fatalf("library without cover should NOT advertise Primary, got %#v", emptyTags)
	}
}
