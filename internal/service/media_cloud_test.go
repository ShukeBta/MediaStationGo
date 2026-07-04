package service

import (
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestDeleteLibraryHardDeletesLibraryRoots(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	rootA := filepath.Join(t.TempDir(), "movies-a")
	rootB := filepath.Join(t.TempDir(), "movies-b")
	lib := &model.Library{Name: "电影", Path: rootA, Type: "movie", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), lib, []model.LibraryRoot{
		{Path: rootA, Enabled: true, SortOrder: 0},
		{Path: rootB, Enabled: true, SortOrder: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{
		LibraryID:     lib.ID,
		LibraryRootID: lib.Roots[0].ID,
		Title:         "测试电影",
		Path:          filepath.Join(rootA, "movie.mkv"),
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	if err := svc.DeleteLibrary(t.Context(), lib.ID); err != nil {
		t.Fatal(err)
	}

	var rootCount int64
	if err := db.Unscoped().Model(&model.LibraryRoot{}).Where("library_id = ?", lib.ID).Count(&rootCount).Error; err != nil {
		t.Fatal(err)
	}
	if rootCount != 0 {
		t.Fatalf("library roots should be hard deleted, count=%d", rootCount)
	}
	var visibleLibraryCount int64
	if err := db.Model(&model.Library{}).Where("id = ?", lib.ID).Count(&visibleLibraryCount).Error; err != nil {
		t.Fatal(err)
	}
	if visibleLibraryCount != 0 {
		t.Fatalf("deleted library should not remain visible, count=%d", visibleLibraryCount)
	}
}

func TestDeleteCloudLibraryPurgesMountWithoutRecycleBin(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "OpenList · 剑来", Path: "cloud://openlist/Anime/JianLai", Type: "anime", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.LibraryRoot{
		LibraryID: lib.ID,
		Name:      "JianLai",
		Path:      lib.Path,
		Enabled:   true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{
		LibraryID: lib.ID,
		Title:     "剑来",
		Path:      "cloud://openlist/Anime/JianLai/Season 1/01.mkv",
		STRMURL:   "/api/cloud/play/openlist?ref=/Anime/JianLai/Season%201/01.mkv",
	}); err != nil {
		t.Fatal(err)
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	if err := svc.DeleteLibrary(t.Context(), lib.ID); err != nil {
		t.Fatal(err)
	}

	var mediaCount int64
	if err := db.Unscoped().Model(&model.Media{}).Where("library_id = ?", lib.ID).Count(&mediaCount).Error; err != nil {
		t.Fatal(err)
	}
	if mediaCount != 0 {
		t.Fatalf("cloud mount media should be purged, count=%d", mediaCount)
	}
	recycle, err := svc.ListRecycleBin(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(recycle) != 0 {
		t.Fatalf("cloud mount removal must not populate recycle bin: %#v", recycle)
	}
	var libCount int64
	if err := db.Unscoped().Model(&model.Library{}).Where("id = ?", lib.ID).Count(&libCount).Error; err != nil {
		t.Fatal(err)
	}
	if libCount != 0 {
		t.Fatalf("cloud mount library should be purged, count=%d", libCount)
	}
	var rootCount int64
	if err := db.Unscoped().Model(&model.LibraryRoot{}).Where("library_id = ?", lib.ID).Count(&rootCount).Error; err != nil {
		t.Fatal(err)
	}
	if rootCount != 0 {
		t.Fatalf("cloud mount roots should be purged, count=%d", rootCount)
	}
}

func TestMediaUpsertBackfillsExternalIDsForPendingCloudRows(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	path := "cloud://openlist/国漫/折腰 (2025) {tmdb-296753}/Season 1/折腰.S01E01.mkv"
	if err := repos.DB.Create(&model.Media{
		LibraryID:    "cloud-tv",
		Title:        "折腰",
		Path:         path,
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{
		LibraryID:    "cloud-tv",
		Title:        "折腰",
		Path:         path,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       296753,
		Year:         2025,
		ScrapeStatus: "pending",
	}); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", path).Error; err != nil {
		t.Fatal(err)
	}
	if got.TMDbID != 296753 || got.Year != 2025 || got.ScrapeStatus != "pending" {
		t.Fatalf("pending cloud row was not backfilled correctly: tmdb=%d year=%d status=%q", got.TMDbID, got.Year, got.ScrapeStatus)
	}
}

func TestMediaUpsertCorrectsCloudExternalIDConflicts(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	path := "cloud://openlist/国产剧/折腰 (2025) {tmdb-296753}/Season 1/折腰.S01E01.mkv"
	if err := repos.DB.Create(&model.Media{
		LibraryID:    "cloud-tv",
		Title:        "折腰",
		Path:         path,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       220269,
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.Media.Upsert(t.Context(), &model.Media{
		LibraryID:    "cloud-tv",
		Title:        "折腰",
		Path:         path,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       296753,
		Year:         2025,
		ScrapeStatus: "pending",
	}); err != nil {
		t.Fatal(err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", path).Error; err != nil {
		t.Fatal(err)
	}
	if got.TMDbID != 296753 || got.ScrapeStatus != "pending" {
		t.Fatalf("cloud external id conflict was not corrected: tmdb=%d status=%q", got.TMDbID, got.ScrapeStatus)
	}
}

func TestRepairCloudPathMetadataBackfillsExistingPlaceholders(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	path := "cloud://openlist/动画电影/雄狮少年2 (2024) {tmdb-1154478}/雄狮少年2 (2024) - 2160p.WEB-DL.H.265.DDP 5.1-ADWeb.mp4"
	if err := repos.DB.Create(&model.Media{
		LibraryID:    "cloud-movie",
		Title:        "雄狮少年2 adweb",
		Path:         path,
		ScrapeStatus: "no_match",
	}).Error; err != nil {
		t.Fatal(err)
	}
	container := &Container{Repo: repos, Log: zap.NewNop()}
	repaired, err := container.RepairCloudPathMetadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if repaired != 1 {
		t.Fatalf("repaired = %d, want 1", repaired)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", path).Error; err != nil {
		t.Fatal(err)
	}
	if got.TMDbID != 1154478 || got.Year != 2024 || got.Title != "雄狮少年2" || got.ScrapeStatus != "pending" {
		t.Fatalf("placeholder was not repaired: title=%q tmdb=%d year=%d status=%q", got.Title, got.TMDbID, got.Year, got.ScrapeStatus)
	}
}

func TestRepairCloudPathMetadataCorrectsConflictingMatchedID(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	path := "cloud://openlist/国产剧/折腰 (2025) {tmdb-296753}/Season 1/折腰.S01E01.mkv"
	if err := repos.DB.Create(&model.Media{
		LibraryID:    "cloud-tv",
		Title:        "折腰",
		Path:         path,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       220269,
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}
	container := &Container{Repo: repos, Log: zap.NewNop()}
	repaired, err := container.RepairCloudPathMetadata(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if repaired != 1 {
		t.Fatalf("repaired = %d, want 1", repaired)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", path).Error; err != nil {
		t.Fatal(err)
	}
	if got.TMDbID != 296753 || got.ScrapeStatus != "pending" {
		t.Fatalf("conflicting matched id was not repaired: tmdb=%d status=%q", got.TMDbID, got.ScrapeStatus)
	}
}

func TestSoftDeleteCloudMediaPurgesRecordWithoutRecycleBin(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{
		Base:    model.Base{ID: "cloud-media"},
		Title:   "网盘电影",
		Path:    "cloud://openlist/电影/Movie.mkv",
		STRMURL: "/api/cloud/play/openlist?ref=/电影/Movie.mkv",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	if err := svc.SoftDelete(t.Context(), media.ID); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Unscoped().Model(&model.Media{}).Where("id = ?", media.ID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("cloud media should be purged, count=%d", count)
	}
	recycle, err := svc.ListRecycleBin(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(recycle) != 0 {
		t.Fatalf("cloud media removal must not populate recycle bin: %#v", recycle)
	}
}
