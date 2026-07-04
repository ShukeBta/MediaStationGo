package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestOrganizeMediaReDetectsSeasonFromPath(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "Incoming", "Some Show", "Season 02")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "Some Show - EP03.mkv")
	if err := os.WriteFile(source, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "TV", Path: root, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Some Show",
		Path:         source,
		Container:    "mkv",
		SeasonNum:    1,
		EpisodeNum:   3,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	dst, err := organizer.OrganizeMedia(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Some Show", "Season 02", "Some Show - S02E03.mkv")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file missing: %v", err)
	}
	var refreshed model.Media
	if err := db.First(&refreshed, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.SeasonNum != 2 || refreshed.EpisodeNum != 3 || refreshed.Path != want {
		t.Fatalf("unexpected refreshed media: %+v", refreshed)
	}
}

func TestOrganizeMediaUsesEpisodeNFOSeason(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "Incoming", "Some Show")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "Some Show - EP03.mkv")
	if err := os.WriteFile(source, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}
	nfo := `<episodedetails><title>第三集</title><season>2</season><episode>3</episode></episodedetails>`
	if err := os.WriteFile(nfoPath(source), []byte(nfo), 0o644); err != nil {
		t.Fatal(err)
	}

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "TV", Path: root, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Some Show",
		Path:         source,
		Container:    "mkv",
		SeasonNum:    1,
		EpisodeNum:   3,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	dst, err := organizer.OrganizeMedia(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "Some Show", "Season 02", "Some Show - S02E03.mkv")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}
}

func TestOrganizeMediaAddsTypeRootForGenericMediaRoot(t *testing.T) {
	root := t.TempDir()
	mediaRoot := filepath.Join(root, "media")
	sourceDir := filepath.Join(root, "downloads", "国产剧")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "Some Show S01E02.mkv")
	if err := os.WriteFile(source, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "organizer.smart_classify", "true"); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "媒体库", Path: mediaRoot, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Some Show",
		Path:         source,
		Container:    "mkv",
		Countries:    "CN",
		SeasonNum:    1,
		EpisodeNum:   2,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	dst, err := organizer.OrganizeMedia(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(mediaRoot, "电视剧", "国产剧", "Some Show", "Season 01", "Some Show - S01E02.mkv")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file missing: %v", err)
	}
}

func TestOrganizeMediaDoesNotRepeatCategoryWhenLibraryIsCategoryRoot(t *testing.T) {
	root := t.TempDir()
	libraryRoot := filepath.Join(root, "media", "电视剧", "国产剧")
	sourceDir := filepath.Join(root, "downloads", "国产剧")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "Some Show S01E02.mkv")
	if err := os.WriteFile(source, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "organizer.smart_classify", "true"); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "国产剧", Path: libraryRoot, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Some Show",
		Path:         source,
		Container:    "mkv",
		Countries:    "CN",
		SeasonNum:    1,
		EpisodeNum:   2,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	dst, err := organizer.OrganizeMedia(t.Context(), media.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(libraryRoot, "Some Show", "Season 01", "Some Show - S01E02.mkv")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}
	repeated := filepath.Join(libraryRoot, "国产剧", "Some Show", "Season 01", "Some Show - S01E02.mkv")
	if _, err := os.Stat(repeated); !os.IsNotExist(err) {
		t.Fatalf("repeated category path exists or stat failed unexpectedly: %v", err)
	}
}

func TestOrganizeMediaTreatsCategoryLibraryAsCollectionRoot(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "media")
	libraryRoot := filepath.Join(dest, "电视剧", "欧美剧")
	sourceDir := filepath.Join(root, "downloads")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "Some Show S01E02.mkv")
	if err := os.WriteFile(source, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	lib := model.Library{Name: "欧美剧", Path: libraryRoot, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Some Show",
		Path:         source,
		Container:    "mkv",
		SeasonNum:    1,
		EpisodeNum:   2,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	dst, err := organizer.OrganizeMediaWithOptions(t.Context(), media.ID, OrganizeOptions{
		MediaType:     "tv",
		MediaCategory: "国产剧",
		TransferMode:  TransferCopy,
	})
	if err != nil {
		t.Fatal(err)
	}
	targetRoot := filepath.Join(dest, "电视剧", "国产剧")
	want := filepath.Join(targetRoot, "Some Show", "Season 01", "Some Show - S01E02.mkv")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file missing: %v", err)
	}
	nested := filepath.Join(libraryRoot, "国产剧", "Some Show", "Season 01", "Some Show - S01E02.mkv")
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("must not nest corrected category under current library, stat err=%v", err)
	}

	var created model.Library
	if err := db.Where("path = ?", targetRoot).First(&created).Error; err != nil {
		t.Fatalf("corrected category library should be auto-created: %v", err)
	}
	var refreshed model.Media
	if err := db.First(&refreshed, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.LibraryID != created.ID {
		t.Fatalf("library_id = %q, want auto-created library %q", refreshed.LibraryID, created.ID)
	}
}

func TestOrganizeLibrarySkipsFilesAlreadyInsideLibrary(t *testing.T) {
	root := t.TempDir()
	libraryRoot := filepath.Join(root, "media", "电视剧", "国产剧")
	sourceDir := filepath.Join(libraryRoot, "Existing Show")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "Existing Show S01E02.mkv")
	if err := os.WriteFile(source, []byte("episode"), 0o644); err != nil {
		t.Fatal(err)
	}

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	if err := repos.Setting.Set(t.Context(), "organizer.smart_classify", "true"); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "国产剧", Path: libraryRoot, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Existing Show",
		Path:         source,
		Container:    "mkv",
		Countries:    "CN",
		SeasonNum:    1,
		EpisodeNum:   2,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	result, err := organizer.OrganizeLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Organized != 0 || result.Skipped != 1 {
		t.Fatalf("result = %+v, want organized=0 skipped=1", result)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("source should remain untouched: %v", err)
	}
	var refreshed model.Media
	if err := db.First(&refreshed, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if refreshed.Path != source {
		t.Fatalf("media path = %q, want unchanged %q", refreshed.Path, source)
	}
}
