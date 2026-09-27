package service

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyLibraryViewExposesFolderCoverTag(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Base: model.Base{ID: "lib-movies"}, Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	base := time.Now()
	rows := []model.Media{
		{
			Base:      model.Base{ID: "media-old", CreatedAt: base, UpdatedAt: base},
			LibraryID: lib.ID,
			Title:     "Old",
			Path:      "/media/movies/old.mkv",
			PosterURL: "https://img.example/old.jpg",
		},
		{
			Base:        model.Base{ID: "media-new", CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute)},
			LibraryID:   lib.ID,
			Title:       "New",
			Path:        "/media/movies/new.mkv",
			PosterURL:   "https://img.example/new.jpg",
			BackdropURL: "https://img.example/new-backdrop.jpg",
		},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	artworks, err := svc.FolderCoverArtwork(t.Context(), lib.ID, "Primary", 4)
	if err != nil {
		t.Fatalf("folder artworks: %v", err)
	}
	if len(artworks) != 2 || artworks[0].URL != "https://img.example/new.jpg" || artworks[1].URL != "https://img.example/old.jpg" {
		t.Fatalf("unexpected folder artworks: %#v", artworks)
	}
	if artworks[0].ImageType != "Primary" || artworks[0].Tag != "media-new" {
		t.Fatalf("unexpected first artwork metadata: %#v", artworks[0])
	}
	tag := svc.FolderCoverTag(t.Context(), lib.ID, "Primary")
	if tag != "633957117b943d361c7f31f9eeca792c" {
		t.Fatalf("folder tag = %q, want legacy proxy tag", tag)
	}

	view := svc.libraryAsView(t.Context(), &lib)
	tags, ok := view["ImageTags"].(map[string]string)
	if !ok || tags["Primary"] != tag {
		t.Fatalf("library view ImageTags = %#v, want Primary %q", view["ImageTags"], tag)
	}
	if _, ok := view["PrimaryImageTag"]; ok {
		t.Fatalf("PrimaryImageTag must not be set for generated folder cover: %#v", view["PrimaryImageTag"])
	}
	if _, ok := view["PrimaryImageItemId"]; ok {
		t.Fatalf("PrimaryImageItemId must not be set for generated folder cover: %#v", view["PrimaryImageItemId"])
	}
	if view["PrimaryImageAspectRatio"] != 16.0/9.0 {
		t.Fatalf("PrimaryImageAspectRatio = %#v, want 16/9", view["PrimaryImageAspectRatio"])
	}
}

func TestEmbyFolderCoverArtworkPrefersBackdropForBackdropRequests(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Base: model.Base{ID: "lib-backdrop"}, Name: "Backdrop", Path: "/media/backdrop", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{
		Base:        model.Base{ID: "media-1"},
		LibraryID:   lib.ID,
		Title:       "Show",
		Path:        "/media/tv/show.mkv",
		PosterURL:   "https://img.example/poster.jpg",
		BackdropURL: "https://img.example/backdrop.jpg",
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	artworks, err := svc.FolderCoverArtwork(t.Context(), lib.ID, "Backdrop", 4)
	if err != nil {
		t.Fatalf("folder artworks: %v", err)
	}
	if len(artworks) != 1 || artworks[0].URL != "https://img.example/backdrop.jpg" {
		t.Fatalf("unexpected backdrop artwork: %#v", artworks)
	}
	if artworks[0].ImageType != "Backdrop" || artworks[0].Tag != "media-1-bd" {
		t.Fatalf("unexpected backdrop metadata: %#v", artworks[0])
	}
}

func TestEmbyFolderCoverArtworkUsesGeneratedImagesWhenRealArtworkIsMissing(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Base: model.Base{ID: "lib-generated"}, Name: "其他媒体", Path: "/media/other", Type: "movie", Enabled: true, GenerateArtwork: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:                   model.Base{ID: "media-generated", UpdatedAt: time.Now()},
		LibraryID:              lib.ID,
		Title:                  "Generated",
		Path:                   "/media/other/generated.mkv",
		GeneratedPosterURL:     "/data/generated-artwork/media-generated/primary.jpg",
		GeneratedBackdropURL:   "/data/generated-artwork/media-generated/backdrop.jpg",
		GeneratedArtworkStatus: GeneratedArtworkStatusCompleted,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	artworks, err := svc.FolderCoverArtwork(t.Context(), lib.ID, "Primary", 4)
	if err != nil {
		t.Fatalf("folder artworks: %v", err)
	}
	if len(artworks) != 1 || artworks[0].URL != media.GeneratedPosterURL || artworks[0].Tag == media.ID {
		t.Fatalf("generated folder artwork = %#v", artworks)
	}
	if raw, err := svc.ImageURL(t.Context(), media.ID, "Backdrop"); err != nil || raw != media.GeneratedBackdropURL {
		t.Fatalf("generated backdrop = %q, err = %v", raw, err)
	}
}

func TestEmbyFolderCoverArtworkUsesSeriesForAnimeLibraries(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Base: model.Base{ID: "lib-anime"}, Name: "Anime", Path: "/media/anime", Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	base := time.Now()
	rows := []model.Media{
		{
			Base:      model.Base{ID: "anime-a-1", CreatedAt: base.Add(4 * time.Minute), UpdatedAt: base.Add(4 * time.Minute)},
			LibraryID: lib.ID,
			Title:     "Anime A",
			Path:      "/media/anime/a-1.mkv",
			SeriesID:  "series-a",
			PosterURL: "https://img.example/anime-a.jpg",
		},
		{
			Base:      model.Base{ID: "anime-a-2", CreatedAt: base.Add(3 * time.Minute), UpdatedAt: base.Add(3 * time.Minute)},
			LibraryID: lib.ID,
			Title:     "Anime A",
			Path:      "/media/anime/a-2.mkv",
			SeriesID:  "series-a",
			PosterURL: "https://img.example/anime-a.jpg",
		},
		{
			Base:      model.Base{ID: "anime-c-1", CreatedAt: base.Add(2 * time.Minute), UpdatedAt: base.Add(2 * time.Minute)},
			LibraryID: lib.ID,
			Title:     "Anime C",
			Path:      "/media/anime/c-1.mkv",
			SeriesID:  "series-c",
			PosterURL: "https://img.example/anime-a.jpg",
		},
		{
			Base:      model.Base{ID: "anime-b-1", CreatedAt: base.Add(time.Minute), UpdatedAt: base.Add(time.Minute)},
			LibraryID: lib.ID,
			Title:     "Anime B",
			Path:      "/media/anime/b-1.mkv",
			SeriesID:  "series-b",
			PosterURL: "https://img.example/anime-b.jpg",
		},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	artworks, err := svc.FolderCoverArtwork(t.Context(), lib.ID, "Primary", 4)
	if err != nil {
		t.Fatalf("folder artworks: %v", err)
	}
	if len(artworks) != 2 {
		t.Fatalf("anime folder artworks = %#v, want two series-level covers", artworks)
	}
	if artworks[0].MediaID != "series-a" || artworks[0].Tag != "series-a" || artworks[0].URL != "https://img.example/anime-a.jpg" {
		t.Fatalf("unexpected first anime artwork: %#v", artworks[0])
	}
	if artworks[1].MediaID != "series-b" || artworks[1].Tag != "series-b" || artworks[1].URL != "https://img.example/anime-b.jpg" {
		t.Fatalf("unexpected second anime artwork: %#v", artworks[1])
	}
}
