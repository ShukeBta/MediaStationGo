package service

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestGroupMediaVersionsMergesEpisodeByExternalIDAcrossLibraries(t *testing.T) {
	local := model.Media{
		LibraryID:  "local-tv",
		Title:      "折腰",
		Path:       "/media/国产剧/折腰 (2025)/Season 1/折腰.S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
		TMDbID:     296753,
		SizeBytes:  100,
		PosterURL:  "https://image.tmdb.org/t/p/w500/poster.jpg",
	}
	cloud := model.Media{
		LibraryID:  "cloud-tv",
		Title:      "折腰",
		Path:       "cloud://openlist/国产剧/折腰 (2025) {tmdb-296753}/Season 1/折腰.S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
		TMDbID:     296753,
		SizeBytes:  200,
		STRMURL:    "/api/cloud/play/openlist?ref=/国产剧/折腰/01.mkv",
	}

	grouped := groupMediaVersions([]model.Media{local, cloud})
	if len(grouped) != 1 {
		t.Fatalf("grouped len = %d, want 1: %#v", len(grouped), grouped)
	}
	if len(grouped[0].Versions) != 2 {
		t.Fatalf("versions len = %d, want 2: %#v", len(grouped[0].Versions), grouped[0].Versions)
	}
	if grouped[0].Media.Path != local.Path {
		t.Fatalf("local version should remain primary, got %q want %q", grouped[0].Media.Path, local.Path)
	}
	if grouped[0].Versions[0].Path != local.Path || grouped[0].Versions[1].Path != cloud.Path {
		t.Fatalf("versions should be ordered local before cloud, got %#v", grouped[0].Versions)
	}
}

func TestGroupMediaVersionsMergesMovieEncodingVariants(t *testing.T) {
	hd := model.Media{
		LibraryID:    "movies",
		Title:        "Inception 2010 1080p BluRay x264",
		Path:         "/media/Movies/Inception.2010.1080p.BluRay.x264.mkv",
		Year:         2010,
		SizeBytes:    100,
		VideoCodec:   "h264",
		ScrapeStatus: "pending",
	}
	uhd := model.Media{
		LibraryID:    "movies",
		Title:        "Inception 2010 2160p UHD BluRay x265",
		Path:         "/media/Movies/Inception.2010.2160p.UHD.BluRay.x265.mkv",
		Year:         2010,
		SizeBytes:    200,
		VideoCodec:   "hevc",
		ScrapeStatus: "pending",
	}

	grouped := groupMediaVersions([]model.Media{hd, uhd})
	if len(grouped) != 1 {
		t.Fatalf("grouped len = %d, want 1: %#v", len(grouped), grouped)
	}
	if len(grouped[0].Versions) != 2 {
		t.Fatalf("versions len = %d, want 2: %#v", len(grouped[0].Versions), grouped[0].Versions)
	}
	if grouped[0].Media.Path != uhd.Path {
		t.Fatalf("larger 2160p version should be primary, got %q want %q", grouped[0].Media.Path, uhd.Path)
	}
}

func TestGroupMediaVersionsRequiresExplicitKeyAfterAITitleCleanup(t *testing.T) {
	first := model.Media{
		Base: model.Base{ID: "cleaned-a"}, LibraryID: "other", Title: "同名作品",
		Path: "/media/other/a.mp4", ScrapeStatus: MediaScrapeStatusTitleCleaned,
		TitleCleanupVersion: currentMediaTitleCleanupVersion,
	}
	second := model.Media{
		Base: model.Base{ID: "cleaned-b"}, LibraryID: "other", Title: "同名作品",
		Path: "/media/other/b.mp4", ScrapeStatus: MediaScrapeStatusTitleCleaned,
		TitleCleanupVersion: currentMediaTitleCleanupVersion,
	}
	if grouped := groupMediaVersions([]model.Media{first, second}); len(grouped) != 2 {
		t.Fatalf("standalone cleaned items must not merge by title: %#v", grouped)
	}
	first.VersionGroupKey = "explicit-version-group"
	second.VersionGroupKey = "explicit-version-group"
	grouped := groupMediaVersions([]model.Media{first, second})
	if len(grouped) != 1 || len(grouped[0].Versions) != 2 {
		t.Fatalf("explicitly grouped versions should merge: %#v", grouped)
	}
}

func TestGroupMediaVersionsCleansCodecPunctuation(t *testing.T) {
	web := model.Media{
		LibraryID:    "movies",
		Title:        "Everything Everywhere All at Once 2022 WEB-DL H.265 DDP5.1",
		Path:         "/media/Movies/Everything.Everywhere.All.At.Once.2022.2160p.WEB-DL.H.265.DDP5.1-GRP.mkv",
		SizeBytes:    200,
		VideoCodec:   "hevc",
		ScrapeStatus: "pending",
	}
	bluray := model.Media{
		LibraryID:    "movies",
		Title:        "Everything Everywhere All at Once 2022 BluRay x264 DTS",
		Path:         "/media/Movies/Everything.Everywhere.All.At.Once.2022.1080p.BluRay.x264.DTS-GRP.mkv",
		SizeBytes:    100,
		VideoCodec:   "h264",
		ScrapeStatus: "pending",
	}

	grouped := groupMediaVersions([]model.Media{web, bluray})
	if len(grouped) != 1 {
		t.Fatalf("grouped len = %d, want 1: %#v", len(grouped), grouped)
	}
	if len(grouped[0].Versions) != 2 {
		t.Fatalf("versions len = %d, want 2: %#v", len(grouped[0].Versions), grouped[0].Versions)
	}
	if grouped[0].Media.Path != web.Path {
		t.Fatalf("larger WEB-DL version should be primary, got %q want %q", grouped[0].Media.Path, web.Path)
	}
}

func TestListMediaVisibleGroupedPaginatesAfterVersionGrouping(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := []model.Media{
		{
			Base:      model.Base{CreatedAt: now.Add(2 * time.Hour), UpdatedAt: now.Add(2 * time.Hour)},
			LibraryID: lib.ID,
			Title:     "Inception 2010 2160p UHD BluRay x265",
			Path:      "/media/movies/Inception.2010.2160p.UHD.BluRay.x265.mkv",
			TMDbID:    27205,
			Year:      2010,
			Width:     3840,
			Height:    2160,
			SizeBytes: 200,
		},
		{
			Base:      model.Base{CreatedAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Hour)},
			LibraryID: lib.ID,
			Title:     "Inception 2010 1080p BluRay x264",
			Path:      "/media/movies/Inception.2010.1080p.BluRay.x264.mkv",
			TMDbID:    27205,
			Year:      2010,
			Width:     1920,
			Height:    1080,
			SizeBytes: 100,
		},
		{
			Base:      model.Base{CreatedAt: now, UpdatedAt: now},
			LibraryID: lib.ID,
			Title:     "The Matrix 1999 1080p BluRay",
			Path:      "/media/movies/The.Matrix.1999.1080p.BluRay.mkv",
			TMDbID:    603,
			Year:      1999,
			SizeBytes: 90,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	var missingBefore int64
	if err := db.Model(&model.Media{}).Where("media_version_key = '' OR media_version_key IS NULL").Count(&missingBefore).Error; err != nil {
		t.Fatal(err)
	}
	if missingBefore != 3 {
		t.Fatalf("missing persisted keys before request = %d, want 3", missingBefore)
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	page, total, err := svc.ListMediaVisibleGrouped(t.Context(), lib.ID, 1, 1, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("grouped total = %d, want 2", total)
	}
	if len(page) != 1 || len(page[0].Versions) != 2 {
		t.Fatalf("first page should contain merged Inception versions, got %#v", page)
	}
	if page[0].Media.Path != rows[0].Path {
		t.Fatalf("primary version = %q, want %q", page[0].Media.Path, rows[0].Path)
	}
	complete, err := repos.Media.MediaVersionKeysComplete(t.Context(), []string{lib.ID}, repository.MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("request path fell back instead of repairing the persisted SQL projection")
	}
}

func TestListMediaVisibleGroupedUsesPersistedVersionProjection(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	lib := model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := []model.Media{
		{
			Base:      model.Base{CreatedAt: now.Add(2 * time.Hour), UpdatedAt: now.Add(2 * time.Hour)},
			LibraryID: lib.ID, Title: "Inception 2010 2160p UHD BluRay x265",
			Path: "/media/movies/Inception.2010.2160p.UHD.BluRay.x265.mkv", TMDbID: 27205,
			Year: 2010, Width: 3840, Height: 2160, SizeBytes: 200,
		},
		{
			Base:      model.Base{CreatedAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Hour)},
			LibraryID: lib.ID, Title: "Inception 2010 1080p BluRay x264",
			Path: "/media/movies/Inception.2010.1080p.BluRay.x264.mkv", TMDbID: 27205,
			Year: 2010, Width: 1920, Height: 1080, SizeBytes: 100,
		},
		{
			Base:      model.Base{CreatedAt: now, UpdatedAt: now},
			LibraryID: lib.ID, Title: "The Matrix 1999 1080p BluRay",
			Path: "/media/movies/The.Matrix.1999.1080p.BluRay.mkv", TMDbID: 603,
			Year: 1999, SizeBytes: 90,
		},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	var stale int64
	if err := db.Model(&model.Media{}).Where("media_version_key_version <> 1").Count(&stale).Error; err != nil {
		t.Fatal(err)
	}
	if stale != 0 {
		t.Fatalf("persisted version keys stale = %d, want 0", stale)
	}

	page, total, err := svc.ListMediaVisibleGrouped(t.Context(), lib.ID, 1, 1, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(page) != 1 || len(page[0].Versions) != 2 {
		t.Fatalf("persisted grouped page = %#v total=%d", page, total)
	}
	if page[0].Media.Path != rows[0].Path {
		t.Fatalf("persisted primary = %q, want %q", page[0].Media.Path, rows[0].Path)
	}
}

func TestEnsureMediaVersionKeysBackfillsAllLibraries(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	rows := []model.Media{
		{Base: model.Base{ID: "movie-a"}, LibraryID: "library-a", Title: "Movie A", Path: "/media/a.mkv"},
		{Base: model.Base{ID: "movie-b"}, LibraryID: "library-b", Title: "Movie B", Path: "/media/b.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	repaired, err := svc.EnsureMediaVersionKeys(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if repaired != 2 {
		t.Fatalf("repaired = %d, want 2", repaired)
	}
	complete, err := repos.Media.MediaVersionKeysComplete(t.Context(), nil, repository.MediaQueryFilter{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if !complete {
		t.Fatal("global media version projection remains incomplete")
	}
}

func TestSearchMediaVisiblePageGroupedPaginatesAfterVersionGrouping(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	now := time.Now()
	rows := []model.Media{
		{
			Base:      model.Base{CreatedAt: now.Add(time.Hour), UpdatedAt: now.Add(time.Hour)},
			LibraryID: "movies",
			Title:     "Dune 2021 2160p WEB-DL H265",
			Path:      "/media/movies/Dune.2021.2160p.WEB-DL.H265.mkv",
			TMDbID:    438631,
			Year:      2021,
			Width:     3840,
			Height:    2160,
			SizeBytes: 220,
		},
		{
			Base:      model.Base{CreatedAt: now, UpdatedAt: now},
			LibraryID: "movies",
			Title:     "Dune 2021 1080p BluRay x264",
			Path:      "/media/movies/Dune.2021.1080p.BluRay.x264.mkv",
			TMDbID:    438631,
			Year:      2021,
			Width:     1920,
			Height:    1080,
			SizeBytes: 120,
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	page, total, err := svc.SearchMediaVisiblePageGrouped(t.Context(), "Dune", 1, 1, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("grouped total = %d, want 1", total)
	}
	if len(page) != 1 || len(page[0].Versions) != 2 {
		t.Fatalf("search page should contain merged Dune versions, got %#v", page)
	}
	if page[0].Media.Path != rows[0].Path {
		t.Fatalf("primary version = %q, want %q", page[0].Media.Path, rows[0].Path)
	}
}

func TestSearchMediaVisibleSeriesPagePaginatesWorksAndKeepsExactMatchFirst(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Title: "黑衣人归来", Path: "/media/movies/returns/movie.mkv"},
		{LibraryID: lib.ID, Title: "黑衣人", Path: "/media/movies/men-in-black/mib-4k.mkv", TMDbID: 607, Width: 3840},
		{LibraryID: lib.ID, Title: "黑衣人", Path: "/media/movies/men-in-black/mib-1080p.mkv", TMDbID: 607, Width: 1920},
		{LibraryID: lib.ID, Title: "黑衣人：全球追缉", Path: "/media/movies/international/movie.mkv"},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	firstPage, total, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "黑衣人", 1, 1, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 3 {
		t.Fatalf("work total = %d, want 3", total)
	}
	if len(firstPage) != 1 || firstPage[0].Rep.Title != "黑衣人" || firstPage[0].Count != 2 {
		t.Fatalf("first work should be exact-title grouped versions, got %#v", firstPage)
	}
	secondPage, secondTotal, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "黑衣人", 2, 1, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if secondTotal != total || len(secondPage) != 1 || secondPage[0].Rep.Title == "黑衣人" {
		t.Fatalf("second work page = %#v total=%d", secondPage, secondTotal)
	}
}
