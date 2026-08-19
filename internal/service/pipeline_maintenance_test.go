package service

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestPipelineMaintenanceRepairMovieExtras(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	svc := NewPipelineMaintenanceService(zap.NewNop(), repos)

	lib, root := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Movie", Path: "cloud://openlist/115/movie/Movie/Movie.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "PV", Path: "cloud://openlist/115/movie/Movie/Extras/PV.mp4"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	main := rows[0]
	extra := rows[1]

	result, err := svc.RepairMovieExtras(t.Context(), main.ID, PipelineMaintenanceTarget{
		Category:         "movie",
		LibraryID:        lib.ID,
		RootID:           root.ID,
		RootOpenListPath: "/115/movie",
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.Status != "success" || result.Updated != 1 || result.MediaCount != 2 || result.OpenListHidePath != "/115/movie/Movie" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !slices.Equal(result.OpenListHidePatterns, []string{"^Extras$"}) {
		t.Fatalf("hide patterns = %#v", result.OpenListHidePatterns)
	}

	var stored model.Media
	if err := db.Unscoped().Where("id = ?", extra.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if !stored.DeletedAt.Valid {
		t.Fatal("extra media should be soft-deleted")
	}
}

func TestPipelineMaintenanceRepairEpisodeVisibility(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	svc := NewPipelineMaintenanceService(zap.NewNop(), repos)

	lib, root := createPipelineMaintenanceRoot(t, db, "anime", "/115/anime")
	created := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", Path: "cloud://openlist/115/anime/Show/S01E01.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", Path: "cloud://openlist/115/anime/Show/S01E02.mkv"},
	}
	if err := db.Create(&created).Error; err != nil {
		t.Fatal(err)
	}
	first := created[0]

	result, err := svc.RepairEpisodeVisibility(t.Context(), first.ID, PipelineMaintenanceTarget{
		Category:         "anime",
		LibraryID:        lib.ID,
		RootID:           root.ID,
		RootOpenListPath: "/115/anime",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || result.Updated != 2 || result.MediaCount != 2 {
		t.Fatalf("unexpected result: %#v", result)
	}

	var rows []model.Media
	if err := db.Where("library_id = ?", lib.ID).Order("path").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d", len(rows))
	}
	if rows[0].SeasonNum != 1 || rows[0].EpisodeNum != 1 || rows[0].RelativePath != "Show/S01E01.mkv" {
		t.Fatalf("first row not repaired: %#v", rows[0])
	}
	if rows[1].SeasonNum != 1 || rows[1].EpisodeNum != 2 || rows[1].RelativePath != "Show/S01E02.mkv" {
		t.Fatalf("second row not repaired: %#v", rows[1])
	}
}

func TestPipelineMaintenancePruneDeletedMedia(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	svc := NewPipelineMaintenanceService(zap.NewNop(), repos)

	lib, root := createPipelineMaintenanceRoot(t, db, "adult", "/115/adult")
	created := []model.Media{
		{
			LibraryID:     lib.ID,
			LibraryRootID: root.ID,
			Title:         "Deleted",
			Path:          "cloud://openlist/115/adult/ABF-363/ABF-363.mp4",
		},
		{
			LibraryID:     lib.ID,
			LibraryRootID: root.ID,
			Title:         "Active",
			Path:          "cloud://openlist/115/adult/ABF-363/Active.mp4",
		},
	}
	if err := db.Create(&created).Error; err != nil {
		t.Fatal(err)
	}
	deleted := created[0]
	active := created[1]
	if err := db.Model(&model.Media{}).Where("id = ?", deleted.ID).Update("deleted_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.PruneDeletedMedia(t.Context(), PipelineMaintenanceTarget{
		Category:         "adult",
		LibraryID:        lib.ID,
		RootID:           root.ID,
		RootOpenListPath: "/115/adult",
	}, []string{"/115/adult/ABF-363"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || result.Deleted != 1 || !slices.Equal(result.MediaIDs, []string{deleted.ID}) {
		t.Fatalf("unexpected result: %#v", result)
	}

	var deletedCount int64
	if err := db.Unscoped().Model(&model.Media{}).Where("id = ?", deleted.ID).Count(&deletedCount).Error; err != nil {
		t.Fatal(err)
	}
	if deletedCount != 0 {
		t.Fatal("deleted media should be hard-deleted")
	}
	var activeCount int64
	if err := db.Model(&model.Media{}).Where("id = ?", active.ID).Count(&activeCount).Error; err != nil {
		t.Fatal(err)
	}
	if activeCount != 1 {
		t.Fatal("active media should be kept")
	}
}

func TestPipelineMaintenanceReplaceWorkSourceMovesOnlyOldSeriesSource(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	svc := NewPipelineMaintenanceService(zap.NewNop(), repos)

	lib, root := createPipelineMaintenanceRoot(t, db, "tv", "/115/tv")
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 303143, SeasonNum: 1, EpisodeNum: 1, Path: "cloud://openlist/115/tv/Show-old/Season 01/Show.S01E01.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 303143, SeasonNum: 1, EpisodeNum: 2, Path: "cloud://openlist/115/tv/Show-old/Season 01/Show.S01E02.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 303143, SeasonNum: 1, EpisodeNum: 1, Path: "cloud://openlist/115/tv/Show-new/Season 01/Show.S01E01.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 303143, SeasonNum: 1, EpisodeNum: 2, Path: "cloud://openlist/115/tv/Show-new/Season 01/Show.S01E02.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 999999, SeasonNum: 1, EpisodeNum: 1, Path: "cloud://openlist/115/tv/Other/Season 01/Other.S01E01.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.ReplaceWorkSource(t.Context(), rows[0].ID, rows[2].ID, PipelineMaintenanceTarget{
		Category: "tv", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/tv",
	}, []string{"/115/tv/Show-new"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "success" || result.Removed != 2 || result.Preserved != 2 || result.WorkIdentity != "tmdb:303143" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !slices.Equal(result.RemovedMediaIDs, []string{rows[0].ID, rows[1].ID}) {
		t.Fatalf("removed ids = %#v", result.RemovedMediaIDs)
	}

	var active []model.Media
	if err := db.Order("path").Find(&active).Error; err != nil {
		t.Fatal(err)
	}
	if len(active) != 3 {
		t.Fatalf("active rows = %d, want 3", len(active))
	}
	for _, row := range active {
		if strings.Contains(row.Path, "/Show-old/") {
			t.Fatalf("old source remains active: %s", row.Path)
		}
	}
	var removed []model.Media
	if err := db.Unscoped().Where("id IN ?", result.RemovedMediaIDs).Find(&removed).Error; err != nil {
		t.Fatal(err)
	}
	for _, row := range removed {
		if !row.DeletedAt.Valid || row.DeletionKind != "version" {
			t.Fatalf("old source row not moved to recycle bin: %#v", row)
		}
	}
}

func TestPipelineMaintenanceReplaceWorkSourcePreservesUncoveredEpisodes(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	svc := NewPipelineMaintenanceService(zap.NewNop(), repos)

	lib, root := createPipelineMaintenanceRoot(t, db, "anime", "/115/anime")
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 101172, SeasonNum: 1, EpisodeNum: 1, Path: "cloud://openlist/115/anime/Show-old/Show.S01E01.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 101172, SeasonNum: 1, EpisodeNum: 101, Path: "cloud://openlist/115/anime/Show-old/Show.S01E101.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", TMDbID: 101172, SeasonNum: 1, EpisodeNum: 1, Path: "cloud://openlist/115/anime/Show-upgrade/Show.S01E01.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.ReplaceWorkSource(t.Context(), rows[0].ID, rows[2].ID, PipelineMaintenanceTarget{
		Category: "anime", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/anime",
	}, []string{"/115/anime/Show-upgrade"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Removed != 1 || !slices.Equal(result.RemovedMediaIDs, []string{rows[0].ID}) {
		t.Fatalf("unexpected removed rows: %#v", result)
	}
	if result.Preserved != 2 || !slices.Contains(result.PreservedMediaIDs, rows[1].ID) {
		t.Fatalf("uncovered episode was not preserved: %#v", result)
	}
	var active model.Media
	if err := db.First(&active, "id = ?", rows[1].ID).Error; err != nil {
		t.Fatalf("uncovered episode should remain active: %v", err)
	}
}

func TestPipelineMaintenanceReplaceWorkSourceRejectsSameDirectory(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	svc := NewPipelineMaintenanceService(zap.NewNop(), repos)

	lib, root := createPipelineMaintenanceRoot(t, db, "anime", "/115/anime")
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", BangumiID: 123, SeasonNum: 1, EpisodeNum: 1, Path: "cloud://openlist/115/anime/Show/Show.S01E01.mkv"},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Show", BangumiID: 123, SeasonNum: 1, EpisodeNum: 2, Path: "cloud://openlist/115/anime/Show/Show.S01E02.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	_, err := svc.ReplaceWorkSource(t.Context(), rows[0].ID, rows[1].ID, PipelineMaintenanceTarget{
		Category: "anime", LibraryID: lib.ID, RootID: root.ID, RootOpenListPath: "/115/anime",
	}, []string{"/115/anime/Show"})
	if err == nil || !strings.Contains(err.Error(), "同一目录") {
		t.Fatalf("same-source error = %v", err)
	}
	var active int64
	if err := db.Model(&model.Media{}).Count(&active).Error; err != nil {
		t.Fatal(err)
	}
	if active != 2 {
		t.Fatalf("active rows = %d, want 2", active)
	}
}

func createPipelineMaintenanceRoot(t *testing.T, db *gorm.DB, libraryType, rootPath string) (model.Library, model.LibraryRoot) {
	t.Helper()
	lib := model.Library{Name: libraryType, Path: pipelineOpenListPathToCloudPath(rootPath), Type: libraryType, Enabled: true}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	root := model.LibraryRoot{LibraryID: lib.ID, Name: libraryType, Path: pipelineOpenListPathToCloudPath(rootPath), Enabled: true}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	return lib, root
}
