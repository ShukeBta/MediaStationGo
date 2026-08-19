package service

import (
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestPipelineMaintenanceSearchMigrationCandidatesGroupsWorkItems(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	svc := NewPipelineMaintenanceService(zap.NewNop(), repository.New(db))
	lib, root := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	rows := []model.Media{
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Godzilla", Path: "cloud://openlist/115/movie/Godzilla/Godzilla.mkv", SizeBytes: 900},
		{LibraryID: lib.ID, LibraryRootID: root.ID, Title: "Godzilla PV", Path: "cloud://openlist/115/movie/Godzilla/Extras/PV.mp4", SizeBytes: 100},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.SearchMigrationCandidates(t.Context(), PipelineMigrationSearchRequest{Query: "godzilla", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items=%#v", result.Items)
	}
	item := result.Items[0]
	if item.SourceOpenListPath != "/115/movie/Godzilla" || item.SourceKind != "folder" || item.MediaCount != 2 || item.TotalSize != 1000 {
		t.Fatalf("unexpected candidate: %#v", item)
	}
}

func TestPipelineMaintenanceListsDeletedMediaHideCandidates(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	svc := NewPipelineMaintenanceService(zap.NewNop(), repository.New(db))
	movieLib, movieRoot := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	animeLib, animeRoot := createPipelineMaintenanceRoot(t, db, "anime", "/115/anime")
	rows := []model.Media{
		{LibraryID: movieLib.ID, LibraryRootID: movieRoot.ID, Title: "Movie", Path: "cloud://openlist/115/movie/Movie/Movie.mkv"},
		{LibraryID: animeLib.ID, LibraryRootID: animeRoot.ID, Title: "Episode", Path: "cloud://openlist/115/anime/Show/S01E01.mkv"},
		{LibraryID: movieLib.ID, LibraryRootID: movieRoot.ID, Title: "Movie 4K", Path: "cloud://openlist/115/movie/Movie/Movie.2160p.mkv", DeletionKind: "version"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Model(&model.Media{}).Where("id IN ?", []string{rows[0].ID, rows[1].ID, rows[2].ID}).Update("deleted_at", now).Error; err != nil {
		t.Fatal(err)
	}

	result, err := svc.ListDeletedMediaHideCandidates(t.Context(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 3 {
		t.Fatalf("items=%#v", result.Items)
	}
	byID := make(map[string]PipelineDeletedMediaHideCandidate, len(result.Items))
	for _, item := range result.Items {
		byID[item.MediaID] = item
	}
	if item := byID[rows[0].ID]; item.TargetOpenListPath != "/115/movie/Movie" || item.HidePath != "/115/movie" || item.HidePattern != "^Movie$" {
		t.Fatalf("movie candidate=%#v", item)
	}
	if item := byID[rows[1].ID]; item.TargetOpenListPath != "/115/anime/Show/S01E01.mkv" || item.HidePath != "/115/anime/Show" || item.HidePattern != `^S01E01\.mkv$` {
		t.Fatalf("anime candidate=%#v", item)
	}
	if item := byID[rows[2].ID]; item.TargetOpenListPath != "/115/movie/Movie/Movie.2160p.mkv" || item.HidePath != "/115/movie/Movie" || item.HidePattern != `^Movie\.2160p\.mkv$` {
		t.Fatalf("version candidate=%#v", item)
	}
}

func TestPipelineMaintenanceAppliesCompleteSeriesMigration(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Series{}, &model.Media{}, &model.STRMRecord{})
	svc := NewPipelineMaintenanceService(zap.NewNop(), repository.New(db))
	sourceLib, sourceRoot := createPipelineMaintenanceRoot(t, db, "tv", "/115/tv")
	targetLib, targetRoot := createPipelineMaintenanceRoot(t, db, "anime", "/115/anime")
	series := model.Series{LibraryID: sourceLib.ID, Title: "Show"}
	if err := db.Create(&series).Error; err != nil {
		t.Fatal(err)
	}
	encodedSource := url.QueryEscape("/115/tv/Show")
	rows := []model.Media{
		{
			LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SeriesID: series.ID, Title: "Show", Path: "cloud://openlist/115/tv/Show/S01E01.mkv",
			RelativePath: "Show/S01E01.mkv", STRMURL: "https://example.test/d/" + encodedSource + "%2FS01E01.mkv",
		},
		{
			LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SeriesID: series.ID, Title: "Show", Path: "cloud://openlist/115/tv/Show/S01E02.mkv",
			RelativePath: "Show/S01E02.mkv",
		},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	strm := model.STRMRecord{
		Title: "Show", URL: "https://example.test/d/" + encodedSource + "%2FS01E01.mkv", FilePath: "cloud://openlist/115/tv/Show/S01E01.strm", Protocol: "openlist", MediaID: rows[0].ID,
	}
	if err := db.Create(&strm).Error; err != nil {
		t.Fatal(err)
	}

	req := PipelineMigrationRequest{
		Source: PipelineMigrationSource{
			Category: "tv", LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SourceOpenListPath: "/115/tv/Show", SourceKind: "folder",
		},
		Target: PipelineMaintenanceTarget{
			Category: "anime", LibraryID: targetLib.ID, RootID: targetRoot.ID, RootOpenListPath: "/115/anime",
		},
	}
	validated, err := svc.ValidateMigration(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if validated.TargetOpenListPath != "/115/anime/Show" || validated.MediaCount != 2 || validated.SeriesCount != 1 {
		t.Fatalf("validated=%#v", validated)
	}
	result, err := svc.ApplyMigration(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result != validated {
		t.Fatalf("result=%#v validated=%#v", result, validated)
	}

	var stored []model.Media
	if err := db.Where("id IN ?", []string{rows[0].ID, rows[1].ID}).Order("path").Find(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 || !slices.Equal([]string{stored[0].Path, stored[1].Path}, []string{
		"cloud://openlist/115/anime/Show/S01E01.mkv",
		"cloud://openlist/115/anime/Show/S01E02.mkv",
	}) {
		t.Fatalf("stored=%#v", stored)
	}
	for _, row := range stored {
		if row.LibraryID != targetLib.ID || row.LibraryRootID != targetRoot.ID || !strings.HasPrefix(row.RelativePath, "Show/") {
			t.Fatalf("media not migrated: %#v", row)
		}
	}
	if strings.Contains(stored[0].STRMURL, encodedSource) {
		t.Fatalf("stale media strm url: %s", stored[0].STRMURL)
	}
	var storedSeries model.Series
	if err := db.Where("id = ?", series.ID).First(&storedSeries).Error; err != nil {
		t.Fatal(err)
	}
	if storedSeries.LibraryID != targetLib.ID {
		t.Fatalf("series library=%s", storedSeries.LibraryID)
	}
	var storedSTRM model.STRMRecord
	if err := db.Where("id = ?", strm.ID).First(&storedSTRM).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(storedSTRM.URL, encodedSource) || storedSTRM.FilePath != "cloud://openlist/115/anime/Show/S01E01.strm" {
		t.Fatalf("strm not migrated: %#v", storedSTRM)
	}
}

func TestPipelineMaintenanceWrapsTopLevelFileMigrationInWorkDirectory(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.STRMRecord{})
	svc := NewPipelineMaintenanceService(zap.NewNop(), repository.New(db))
	sourceLib, sourceRoot := createPipelineMaintenanceRoot(t, db, "other", "/115/other")
	targetLib, targetRoot := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	row := model.Media{
		LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, Title: "Sintel", Path: "cloud://openlist/115/other/Sintel.mkv",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	req := PipelineMigrationRequest{
		Source: PipelineMigrationSource{
			Category: "other", LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SourceOpenListPath: "/115/other/Sintel.mkv", SourceKind: "file",
		},
		Target:             PipelineMaintenanceTarget{Category: "movie", LibraryID: targetLib.ID, RootID: targetRoot.ID, RootOpenListPath: "/115/movie"},
		TargetOpenListPath: "/115/movie/Sintel/Sintel.mkv",
	}
	validated, err := svc.ValidateMigration(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if validated.TargetOpenListPath != req.TargetOpenListPath {
		t.Fatalf("validated=%#v", validated)
	}
	result, err := svc.ApplyMigration(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.TargetOpenListPath != req.TargetOpenListPath {
		t.Fatalf("result=%#v", result)
	}
	var stored model.Media
	if err := db.First(&stored, "id = ?", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Path != "cloud://openlist/115/movie/Sintel/Sintel.mkv" || stored.RelativePath != "Sintel/Sintel.mkv" {
		t.Fatalf("stored=%#v", stored)
	}
}

func TestPipelineMaintenanceRejectsTopLevelFileMigrationTarget(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	svc := NewPipelineMaintenanceService(zap.NewNop(), repository.New(db))
	sourceLib, sourceRoot := createPipelineMaintenanceRoot(t, db, "other", "/115/other")
	targetLib, targetRoot := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	row := model.Media{LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, Title: "Sintel", Path: "cloud://openlist/115/other/Sintel.mkv"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	_, err := svc.ValidateMigration(t.Context(), PipelineMigrationRequest{
		Source:             PipelineMigrationSource{LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SourceOpenListPath: "/115/other/Sintel.mkv", SourceKind: "file"},
		Target:             PipelineMaintenanceTarget{Category: "movie", LibraryID: targetLib.ID, RootID: targetRoot.ID, RootOpenListPath: "/115/movie"},
		TargetOpenListPath: "/115/movie/Sintel.mkv",
	})
	if err == nil || !strings.Contains(err.Error(), "one work directory") {
		t.Fatalf("err=%v", err)
	}
}

func TestPipelineMaintenanceRejectsPartialSeriesMigration(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Series{}, &model.Media{})
	svc := NewPipelineMaintenanceService(zap.NewNop(), repository.New(db))
	sourceLib, sourceRoot := createPipelineMaintenanceRoot(t, db, "tv", "/115/tv")
	targetLib, targetRoot := createPipelineMaintenanceRoot(t, db, "anime", "/115/anime")
	series := model.Series{LibraryID: sourceLib.ID, Title: "Show"}
	if err := db.Create(&series).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SeriesID: series.ID, Title: "Show", Path: "cloud://openlist/115/tv/Show/S01E01.mkv"},
		{LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SeriesID: series.ID, Title: "Show", Path: "cloud://openlist/115/tv/Other/S01E02.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	_, err := svc.ValidateMigration(t.Context(), PipelineMigrationRequest{
		Source: PipelineMigrationSource{LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SourceOpenListPath: "/115/tv/Show", SourceKind: "folder"},
		Target: PipelineMaintenanceTarget{Category: "anime", LibraryID: targetLib.ID, RootID: targetRoot.ID, RootOpenListPath: "/115/anime"},
	})
	if err == nil || !strings.Contains(err.Error(), "partial series") {
		t.Fatalf("err=%v", err)
	}
}

func TestPipelineMaintenanceRejectsDeletedTargetPath(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	svc := NewPipelineMaintenanceService(zap.NewNop(), repository.New(db))
	sourceLib, sourceRoot := createPipelineMaintenanceRoot(t, db, "movie", "/115/movie")
	targetLib, targetRoot := createPipelineMaintenanceRoot(t, db, "anime", "/115/anime")
	rows := []model.Media{
		{LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, Title: "Movie", Path: "cloud://openlist/115/movie/Movie/Movie.mkv"},
		{LibraryID: targetLib.ID, LibraryRootID: targetRoot.ID, Title: "Old", Path: "cloud://openlist/115/anime/Movie/Old.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Media{}).Where("id = ?", rows[1].ID).Update("deleted_at", time.Now()).Error; err != nil {
		t.Fatal(err)
	}

	_, err := svc.ValidateMigration(t.Context(), PipelineMigrationRequest{
		Source: PipelineMigrationSource{LibraryID: sourceLib.ID, LibraryRootID: sourceRoot.ID, SourceOpenListPath: "/115/movie/Movie", SourceKind: "folder"},
		Target: PipelineMaintenanceTarget{Category: "anime", LibraryID: targetLib.ID, RootID: targetRoot.ID, RootOpenListPath: "/115/anime"},
	})
	if err == nil || !strings.Contains(err.Error(), "target already exists") {
		t.Fatalf("err=%v", err)
	}
}
