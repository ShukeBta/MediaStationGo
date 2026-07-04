package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestScanCloudLibraryImportsRecursivePlayableMedia(t *testing.T) {
	empty := false
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		if empty {
			return nil, 0
		}
		switch path {
		case "/":
			return []openListTestEntry{
				{Name: "Movies", IsDir: true},
				{Name: "Root.Movie.2024.mkv", Size: 123},
			}, 2
		case "/Movies":
			return []openListTestEntry{
				{Name: "Nested.Show.S01E02.mp4", Size: 456},
			}, 1
		default:
			t.Fatalf("unexpected openlist path %q", path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan cloud: %v", err)
	}
	if res.Visited != 2 || res.Added != 2 {
		t.Fatalf("scan result = %#v, want visited=2 added=2", res)
	}
	var rows []model.Media
	if err := repos.DB.Order("path").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("media rows = %d, want 2: %#v", len(rows), rows)
	}
	if rows[0].Path != "cloud://openlist/Movies/Nested.Show.S01E02.mp4" || !strings.Contains(rows[0].STRMURL, "ref=%2FMovies%2FNested.Show.S01E02.mp4") {
		t.Fatalf("nested media path/strm wrong: path=%q strm=%q", rows[0].Path, rows[0].STRMURL)
	}
	if rows[0].SeasonNum != 1 || rows[0].EpisodeNum != 2 {
		t.Fatalf("nested episode metadata wrong: %#v", rows[0])
	}
	if rows[1].Path != "cloud://openlist/Root.Movie.2024.mkv" || rows[1].STRMURL != "/api/cloud/play/openlist?ref=%2FRoot.Movie.2024.mkv" {
		t.Fatalf("root media path/strm wrong: path=%q strm=%q", rows[0].Path, rows[0].STRMURL)
	}

	res, err = scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("rescan same cloud: %v", err)
	}
	if res.Added != 0 || res.Updated != 0 || res.Skipped != 2 {
		t.Fatalf("same cloud rescan should skip unchanged rows, got %#v", res)
	}

	empty = true
	res, err = scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("rescan cloud: %v", err)
	}
	if res.Removed != 2 {
		t.Fatalf("removed = %d, want 2", res.Removed)
	}
	if got := countMedia(t, repos); got != 0 {
		t.Fatalf("media count after prune = %d, want 0", got)
	}
	var allRows int64
	if err := repos.DB.Unscoped().Model(&model.Media{}).Count(&allRows).Error; err != nil {
		t.Fatal(err)
	}
	if allRows != 0 {
		t.Fatalf("unscoped media count after cloud prune = %d, want 0", allRows)
	}
}

func TestScanOpenListCloudLibraryUsesAPIPaginationBeyondFirstPage(t *testing.T) {
	const totalFiles = 125
	requestedPages := map[int]bool{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/fs/list" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.Header.Get("Authorization") != "openlist-token" {
			t.Fatalf("missing openlist token: %q", r.Header.Get("Authorization"))
		}
		var in struct {
			Path    string `json:"path"`
			Page    int    `json:"page"`
			PerPage int    `json:"per_page"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if in.Path != "/Movies" {
			t.Fatalf("path = %q, want /Movies", in.Path)
		}
		if in.PerPage <= 100 {
			t.Fatalf("per_page = %d, want API pagination larger than legacy 100", in.PerPage)
		}
		requestedPages[in.Page] = true
		effectivePageSize := in.PerPage
		if effectivePageSize > 100 {
			effectivePageSize = 100
		}
		start := (in.Page - 1) * effectivePageSize
		content := []map[string]any{}
		for idx := start; idx < totalFiles && idx < start+effectivePageSize; idx++ {
			content = append(content, map[string]any{
				"name":   fmt.Sprintf("Movie.%03d.mkv", idx+1),
				"size":   int64(1024 + idx),
				"is_dir": false,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    200,
			"message": "success",
			"data": map[string]any{
				"content": content,
				"total":   totalFiles,
			},
		})
	}))
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList · Movies", Path: BuildCloudLibraryPath("openlist", "/Movies", "/Movies"), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan openlist: %v", err)
	}
	if res.Added != totalFiles {
		t.Fatalf("scan result = %#v, want added=%d", res, totalFiles)
	}
	if got := countMedia(t, repos); got != totalFiles {
		t.Fatalf("media count = %d, want %d", got, totalFiles)
	}
	if !requestedPages[1] || !requestedPages[2] {
		t.Fatalf("expected pagination beyond the first 100 entries, got pages %#v", requestedPages)
	}
}

func TestScanCloudLibraryQueuesMissingExistingTrackMetadataBeforeNewFiles(t *testing.T) {
	const newFiles = maxCloudMediaProbeQueuePerScan + 5
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		if path != "/" {
			t.Fatalf("unexpected openlist path %q", path)
		}
		entries := make([]openListTestEntry, 0, newFiles+1)
		for i := 0; i < newFiles; i++ {
			entries = append(entries, openListTestEntry{Name: fmt.Sprintf("New.Movie.%02d.mkv", i), Size: int64(1000 + i)})
		}
		entries = append(entries, openListTestEntry{Name: "Existing.Show.S01E01.mkv", Size: 2048})
		return entries, len(entries)
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	existingPath := "cloud://openlist/Existing.Show.S01E01.mkv"
	if err := repos.DB.Create(&model.Media{
		LibraryID:  lib.ID,
		Title:      "Existing Show",
		Path:       existingPath,
		SizeBytes:  2048,
		Container:  "mkv",
		STRMURL:    "/api/cloud/play/openlist?ref=%2FExisting.Show.S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), NewFFprobeService(&config.Config{}, log), nil)
	scanner.storage = storage

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan cloud: %v", err)
	}
	if res.Added != newFiles || res.Skipped != 1 {
		t.Fatalf("scan result = %#v, want new files added and existing skipped", res)
	}
	foundExistingProbe := false
	for {
		select {
		case task := <-scanner.cloudMediaProbeQueue:
			if task.path == existingPath {
				foundExistingProbe = true
			}
		default:
			if !foundExistingProbe {
				t.Fatal("existing media missing track metadata did not receive probe budget before new files")
			}
			return
		}
	}
}

func TestScanCloudLibraryRefreshesStaleNoMatchDerivedMetadata(t *testing.T) {
	const showDir = "Hntv Spring Festival Gala S01e (2026)"
	const seasonDir = "Season 1"
	const name = "Hntv Spring Festival Gala S01e - S01E202-DD5.QHstudIo.6.4K - 第 202 集.ts"
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		switch path {
		case "/":
			return []openListTestEntry{{Name: showDir, IsDir: true}}, 1
		case "/" + showDir:
			return []openListTestEntry{{Name: seasonDir, IsDir: true}}, 1
		case "/" + showDir + "/" + seasonDir:
			return []openListTestEntry{{Name: name, Size: 1}}, 1
		default:
			t.Fatalf("unexpected openlist path %q", path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": upstream.URL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	lib := model.Library{Name: "OpenList · 综艺", Path: "cloud://openlist", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	ref := "/" + showDir + "/" + seasonDir + "/" + name
	path := "cloud://openlist/" + showDir + "/" + seasonDir + "/" + name
	if err := repos.DB.Create(&model.Media{
		LibraryID:    lib.ID,
		Title:        "hntv spring festival gala s01e",
		Path:         path,
		SizeBytes:    1,
		Container:    "ts",
		STRMURL:      BuildRelativeCloudPlayURL("openlist", ref),
		ScrapeStatus: "no_match",
	}).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)

	res, err := scanner.ScanLibrary(t.Context(), lib.ID)
	if err != nil {
		t.Fatalf("scan cloud: %v", err)
	}
	if res.Updated != 1 || res.Skipped != 0 {
		t.Fatalf("scan result updated=%d skipped=%d, want 1/0: %#v", res.Updated, res.Skipped, res)
	}
	var media model.Media
	if err := repos.DB.First(&media, "path = ?", path).Error; err != nil {
		t.Fatal(err)
	}
	if media.Title != "hntv spring festival gala" || media.SeasonNum != 1 || media.EpisodeNum != 202 || media.ScrapeStatus != "pending" {
		t.Fatalf("stale cloud row was not refreshed: title=%q s=%d e=%d status=%q", media.Title, media.SeasonNum, media.EpisodeNum, media.ScrapeStatus)
	}
}
