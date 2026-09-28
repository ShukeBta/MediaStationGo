package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestEnrichExternalMediaLibraryLinksUsesVisibleProviderMatch(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	rows := []model.Media{
		{Base: model.Base{ID: "visible-provider"}, LibraryID: "library-a", Title: "本地标题", Path: "/a/movie.mp4", TMDbID: 42, Year: 2026},
		{Base: model.Base{ID: "hidden-provider"}, LibraryID: "library-b", Title: "本地标题", Path: "/b/movie.mp4", TMDbID: 42, Year: 2026},
		{Base: model.Base{ID: "visible-title"}, LibraryID: "library-a", Title: "标题命中", Path: "/a/title.mp4", Year: 2025},
		{Base: model.Base{ID: "tv-id-collision"}, LibraryID: "library-a", SeriesID: "series-240", Title: "成龙历险记", Path: "/a/tv.mp4", TMDbID: 240, Year: 2000, SeasonNum: 1, EpisodeNum: 1},
		{Base: model.Base{ID: "movie-year-mismatch"}, LibraryID: "library-a", Title: "同类型错误年份", Path: "/a/wrong-year.mp4", TMDbID: 241, Year: 2001},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	items := []ExternalMediaResult{
		{Title: "外部标题", MediaType: "movie", TMDbID: 42, Year: 2026},
		{Title: "标题命中", Year: 2025},
		{Title: "教父2", MediaType: "movie", TMDbID: 240, Year: 1974},
		{Title: "成龙历险记", MediaType: "tv", TMDbID: 240, Year: 2000},
		{Title: "年份冲突", MediaType: "movie", TMDbID: 241, Year: 1974},
	}
	EnrichExternalMediaLibraryLinks(t.Context(), repos, items, MediaVisibility{
		IncludeNSFW: true, AllowedLibraryIDs: []string{"library-a"},
	})
	if !items[0].InLibrary || items[0].LocalMediaID != "visible-provider" || items[0].LocalLibraryID != "library-a" {
		t.Fatalf("provider match = %#v", items[0])
	}
	if !items[1].InLibrary || items[1].LocalMediaID != "visible-title" {
		t.Fatalf("title fallback = %#v", items[1])
	}
	if items[2].InLibrary || items[2].LocalMediaID != "" {
		t.Fatalf("movie must not match a TV item with the same TMDb number: %#v", items[2])
	}
	if !items[3].InLibrary || items[3].LocalMediaID != "tv-id-collision" {
		t.Fatalf("TV provider match = %#v", items[3])
	}
	if items[4].InLibrary || items[4].LocalMediaID != "" {
		t.Fatalf("known year mismatch must not fall through: %#v", items[4])
	}
}

func TestEnrichExternalMediaLibraryLinksMatchesFD2PPVCodeOnlyForAdultSource(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	rows := []model.Media{
		{
			Base:         model.Base{ID: "fd2-adult"},
			LibraryID:    "library-a",
			Title:        "本地中文标题",
			OriginalName: "fc2ppv-920821",
			Path:         "/adult/fc2ppv920821/main.mp4",
			Year:         2020,
			NSFW:         true,
		},
		{
			Base:         model.Base{ID: "regular-fc2-text"},
			LibraryID:    "library-a",
			Title:        "普通媒体",
			OriginalName: "fc2ppv-930000",
			Path:         "/movies/fc2ppv930000.mp4",
			NSFW:         false,
		},
		{
			Base:         model.Base{ID: "other-adult-source"},
			LibraryID:    "library-a",
			Title:        "其他成人来源",
			OriginalName: "fc2ppv-940000",
			Path:         "/adult/fc2ppv940000.mp4",
			NSFW:         true,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	items := []ExternalMediaResult{
		{Source: "fd2ppv", MediaType: "adult", ProviderID: "920821", Title: "FD2 标题", OriginalName: "FC2-PPV-920821", Year: 2026},
		{Source: "fd2ppv", MediaType: "adult", ProviderID: "930000", Title: "FD2 普通库碰撞", OriginalName: "FC2-PPV-930000"},
		{Source: "javdb", MediaType: "adult", ProviderID: "940000", Title: "其他来源标题", OriginalName: "FC2-PPV-940000"},
	}

	EnrichExternalMediaLibraryLinks(t.Context(), repos, items, MediaVisibility{
		IncludeNSFW: true, AllowedLibraryIDs: []string{"library-a"},
	})

	if !items[0].InLibrary || items[0].LocalMediaID != "fd2-adult" || items[0].LocalLibraryID != "library-a" {
		t.Fatalf("FD2PPV code match = %#v", items[0])
	}
	if items[1].InLibrary || items[1].LocalMediaID != "" {
		t.Fatalf("FD2PPV item must not match a non-adult local row: %#v", items[1])
	}
	if items[2].InLibrary || items[2].LocalMediaID != "" {
		t.Fatalf("non-FD2 source must not use FD2PPV code matching: %#v", items[2])
	}
}
