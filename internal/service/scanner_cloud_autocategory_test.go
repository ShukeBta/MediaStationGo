package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestScanRootCloudLibraryCreatesAutoCategoryLibraries(t *testing.T) {
	empty := false
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		if empty {
			return nil, 0
		}
		switch path {
		case "/":
			return []openListTestEntry{
				{Name: "电视剧", IsDir: true},
				{Name: "电影", IsDir: true},
				{Name: "国漫", IsDir: true},
			}, 3
		case "/电视剧":
			return []openListTestEntry{{Name: "欧美剧", IsDir: true}}, 1
		case "/电视剧/欧美剧":
			return []openListTestEntry{{Name: "The Show", IsDir: true}}, 1
		case "/电视剧/欧美剧/The Show":
			return []openListTestEntry{{Name: "The.Show.S01E01.mkv", Size: 101}}, 1
		case "/电影":
			return []openListTestEntry{{Name: "华语电影", IsDir: true}}, 1
		case "/电影/华语电影":
			return []openListTestEntry{{Name: "Movie.2024.mkv", Size: 202}}, 1
		case "/国漫":
			return []openListTestEntry{{Name: "剑来", IsDir: true}}, 1
		case "/国漫/剑来":
			return []openListTestEntry{{Name: "剑来.S01E01.mkv", Size: 303}}, 1
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
	root := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &root); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)

	res, err := scanner.ScanLibrary(t.Context(), root.ID)
	if err != nil {
		t.Fatalf("scan root cloud: %v", err)
	}
	if res.Visited != 3 || res.Added != 3 {
		t.Fatalf("scan result = %#v, want visited=3 added=3", res)
	}

	libs, err := repos.Library.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	byDisplayDir := map[string]model.Library{}
	for _, lib := range libs {
		if !CloudLibraryAutoCategory(lib) {
			continue
		}
		info, ok := ParseCloudLibraryMount(lib.Path)
		if !ok {
			t.Fatalf("auto category path did not parse: %q", lib.Path)
		}
		byDisplayDir[info.DisplayDir] = lib
	}
	wantTypes := map[string]string{
		"电视剧/欧美剧": "tv",
		"电影/华语电影": "movie",
		"动漫/国漫":   "anime",
	}
	for dir, wantType := range wantTypes {
		lib, ok := byDisplayDir[dir]
		if !ok {
			t.Fatalf("missing auto category library %q; got %#v", dir, byDisplayDir)
		}
		if lib.Type != wantType {
			t.Fatalf("auto category %s type = %s, want %s", dir, lib.Type, wantType)
		}
	}

	var rows []model.Media
	if err := repos.DB.Order("path").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("media rows = %d, want 3", len(rows))
	}
	wantLibraries := map[string]string{
		"cloud://openlist/电视剧/欧美剧/The Show/The.Show.S01E01.mkv": byDisplayDir["电视剧/欧美剧"].ID,
		"cloud://openlist/电影/华语电影/Movie.2024.mkv":               byDisplayDir["电影/华语电影"].ID,
		"cloud://openlist/动漫/国漫/剑来/剑来.S01E01.mkv":             byDisplayDir["动漫/国漫"].ID,
	}
	for _, row := range rows {
		if row.LibraryID != wantLibraries[row.Path] {
			t.Fatalf("%s library_id = %s, want %s", row.Path, row.LibraryID, wantLibraries[row.Path])
		}
	}

	res, err = scanner.ScanLibrary(t.Context(), root.ID)
	if err != nil {
		t.Fatalf("rescan root cloud: %v", err)
	}
	if res.Added != 0 || res.Updated != 0 || res.Skipped != 3 {
		t.Fatalf("rescan should skip unchanged auto-category rows, got %#v", res)
	}
	libs, err = repos.Library.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	autoCount := 0
	for _, lib := range libs {
		if CloudLibraryAutoCategory(lib) {
			autoCount++
		}
	}
	if autoCount != 3 {
		t.Fatalf("auto category library count after rescan = %d, want 3", autoCount)
	}

	empty = true
	res, err = scanner.ScanLibrary(t.Context(), root.ID)
	if err != nil {
		t.Fatalf("empty rescan root cloud: %v", err)
	}
	if res.Removed != 3 {
		t.Fatalf("removed = %d, want 3", res.Removed)
	}
	if got := countMedia(t, repos); got != 0 {
		t.Fatalf("media count after auto-category prune = %d, want 0", got)
	}
}

func TestScanRootCloudAutoCategoryAppendsExistingLibraryRoot(t *testing.T) {
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		switch path {
		case "/":
			return []openListTestEntry{{Name: "电影", IsDir: true}}, 1
		case "/电影":
			return []openListTestEntry{{Name: "华语电影", IsDir: true}}, 1
		case "/电影/华语电影":
			return []openListTestEntry{{Name: "Movie.2024.mkv", Size: 202}}, 1
		default:
			t.Fatalf("unexpected openlist path %q", path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	storage := newOpenListStorageForTest(t, repos, upstream.URL)
	local := model.Library{Name: "华语电影", Path: "/media/电影/华语电影", Type: "movie", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), &local, []model.LibraryRoot{{
		Name:    "华语电影",
		Path:    local.Path,
		Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	root := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &root); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	scanner.SetStorageConfig(storage)

	res, err := scanner.ScanLibrary(t.Context(), root.ID)
	if err != nil {
		t.Fatalf("scan root cloud: %v", err)
	}
	if res.Added != 1 {
		t.Fatalf("added = %d, want 1", res.Added)
	}
	libs, err := repos.Library.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, lib := range libs {
		if CloudLibraryAutoCategory(lib) {
			t.Fatalf("auto category should append to existing library, got extra library %#v", lib)
		}
	}
	roots, err := repos.Library.ListRoots(t.Context(), local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 {
		t.Fatalf("roots = %#v, want local root plus cloud root", roots)
	}
	cloudRoot := roots[1]
	if cloudRoot.Name != "华语电影" || !CloudLibraryAutoCategory(model.Library{Path: cloudRoot.Path}) {
		t.Fatalf("cloud root = %#v, want auto-category 华语电影 root", cloudRoot)
	}
	info, ok := ParseCloudLibraryMount(cloudRoot.Path)
	if !ok || info.DisplayDir != "电影/华语电影" || info.ScanDir != "电影/华语电影" {
		t.Fatalf("cloud root mount = %#v, want display/scan 电影/华语电影", info)
	}
	var media model.Media
	if err := repos.DB.First(&media, "path = ?", "cloud://openlist/电影/华语电影/Movie.2024.mkv").Error; err != nil {
		t.Fatal(err)
	}
	if media.LibraryID != local.ID || media.LibraryRootID != cloudRoot.ID {
		t.Fatalf("media placement = library %s root %s, want %s/%s", media.LibraryID, media.LibraryRootID, local.ID, cloudRoot.ID)
	}
}

func TestScanRootCloudAutoCategoryPreservesFlatScanDir(t *testing.T) {
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		switch path {
		case "/":
			return []openListTestEntry{{Name: "国漫", IsDir: true}}, 1
		case "/国漫":
			return []openListTestEntry{{Name: "剑来", IsDir: true}}, 1
		case "/国漫/剑来":
			return []openListTestEntry{{Name: "剑来.S01E01.mkv", Size: 303}}, 1
		default:
			t.Fatalf("unexpected openlist path %q", path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	storage := newOpenListStorageForTest(t, repos, upstream.URL)
	local := model.Library{Name: "国漫", Path: "/media/动漫/国漫", Type: "anime", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), &local, []model.LibraryRoot{{
		Name:    "国漫",
		Path:    local.Path,
		Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	root := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &root); err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	scanner.SetStorageConfig(storage)

	if _, err := scanner.ScanLibrary(t.Context(), root.ID); err != nil {
		t.Fatalf("scan root cloud: %v", err)
	}
	roots, err := repos.Library.ListRoots(t.Context(), local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 {
		t.Fatalf("roots = %#v, want local root plus flat cloud root", roots)
	}
	cloudRoot := roots[1]
	info, ok := ParseCloudLibraryMount(cloudRoot.Path)
	if !ok || info.DisplayDir != "动漫/国漫" || info.ScanDir != "国漫" {
		t.Fatalf("flat cloud root mount = %#v, want display 动漫/国漫 and scan 国漫", info)
	}
	res, err := scanner.ScanLibraryRoot(t.Context(), local.ID, cloudRoot.ID)
	if err != nil {
		t.Fatalf("scan flat cloud root: %v", err)
	}
	if res.Skipped != 1 && res.Updated != 1 {
		t.Fatalf("flat cloud root rescan = %#v, want existing media refreshed/skipped", res)
	}
}

func TestScanRootCloudAutoCategoryMigratesExistingAutoLibrary(t *testing.T) {
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		switch path {
		case "/":
			return []openListTestEntry{{Name: "电视剧", IsDir: true}}, 1
		case "/电视剧":
			return []openListTestEntry{{Name: "欧美剧", IsDir: true}}, 1
		case "/电视剧/欧美剧":
			return []openListTestEntry{{Name: "The Show", IsDir: true}}, 1
		case "/电视剧/欧美剧/The Show":
			return []openListTestEntry{{Name: "The.Show.S01E01.mkv", Size: 101}}, 1
		default:
			t.Fatalf("unexpected openlist path %q", path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{})
	repos := repository.New(db)
	storage := newOpenListStorageForTest(t, repos, upstream.URL)
	local := model.Library{Name: "欧美剧", Path: "/media/电视剧/欧美剧", Type: "tv", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), &local, []model.LibraryRoot{{
		Name:    "欧美剧",
		Path:    local.Path,
		Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}
	root := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	oldAuto := model.Library{Name: "欧美剧", Path: BuildCloudAutoCategoryLibraryPath("openlist", "电视剧/欧美剧"), Type: "tv", Enabled: true}
	for _, lib := range []*model.Library{&root, &oldAuto} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	mediaPath := "cloud://openlist/电视剧/欧美剧/The Show/The.Show.S01E01.mkv"
	if err := repos.DB.Create(&model.Media{LibraryID: oldAuto.ID, Title: "The Show", Path: mediaPath}).Error; err != nil {
		t.Fatal(err)
	}
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	scanner.SetStorageConfig(storage)

	if _, err := scanner.ScanLibrary(t.Context(), root.ID); err != nil {
		t.Fatalf("scan root cloud: %v", err)
	}
	if old, err := repos.Library.FindByID(t.Context(), oldAuto.ID); err != nil || old != nil {
		t.Fatalf("old auto library = %#v, err=%v; want removed", old, err)
	}
	roots, err := repos.Library.ListRoots(t.Context(), local.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 {
		t.Fatalf("roots = %#v, want local root plus migrated cloud root", roots)
	}
	var media model.Media
	if err := repos.DB.First(&media, "path = ?", mediaPath).Error; err != nil {
		t.Fatal(err)
	}
	if media.LibraryID != local.ID || media.LibraryRootID != roots[1].ID {
		t.Fatalf("migrated media placement = %s/%s, want %s/%s", media.LibraryID, media.LibraryRootID, local.ID, roots[1].ID)
	}
}

func TestScanCloudLibraryListsChildDirectoriesConcurrently(t *testing.T) {
	var active int32
	var maxActive int32
	var releaseOnce sync.Once
	release := make(chan struct{})
	upstream := newOpenListAPIServer(t, func(path string, page, perPage int) ([]openListTestEntry, int) {
		switch path {
		case "/":
			return []openListTestEntry{
				{Name: "A", IsDir: true},
				{Name: "B", IsDir: true},
			}, 2
		case "/A", "/B":
			cur := atomic.AddInt32(&active, 1)
			defer atomic.AddInt32(&active, -1)
			for {
				prev := atomic.LoadInt32(&maxActive)
				if cur <= prev || atomic.CompareAndSwapInt32(&maxActive, prev, cur) {
					break
				}
			}
			if cur >= 2 {
				releaseOnce.Do(func() { close(release) })
			}
			select {
			case <-release:
			case <-time.After(1500 * time.Millisecond):
				t.Errorf("child directory requests were not concurrent")
				return nil, 0
			}
			id := strings.TrimPrefix(path, "/")
			return []openListTestEntry{{Name: fmt.Sprintf("Movie.%s.mkv", id), Size: 123}}, 1
		default:
			t.Errorf("unexpected openlist path %q", path)
			return nil, 0
		}
	})
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file:cloud_scan_concurrent?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Media{}, &model.Setting{}, &model.StorageConfig{}); err != nil {
		t.Fatal(err)
	}
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
	lib := model.Library{Name: "OpenList", Path: "cloud://openlist", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{}
	cfg.App.CloudScanMaxConcurrent = 2
	scanner := NewScannerService(cfg, log, repos, NewHub(log), nil, nil)
	scanner.SetStorageConfig(storage)
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	res, err := scanner.ScanLibrary(ctx, lib.ID)
	if err != nil {
		t.Fatalf("scan cloud: %v", err)
	}
	if got := atomic.LoadInt32(&maxActive); got < 2 {
		t.Fatalf("max concurrent child lists = %d, want >= 2", got)
	}
	if res.Visited != 2 || res.Added != 2 {
		t.Fatalf("scan result = %#v, want visited=2 added=2", res)
	}
}

func newOpenListStorageForTest(t *testing.T, repos *repository.Container, serverURL string) *StorageConfigService {
	t.Helper()
	log := zap.NewNop()
	storage := NewStorageConfigService(log, repos, NewCryptoService("", log))
	if _, err := storage.Save(t.Context(), StorageInput{
		Type: "openlist",
		Config: map[string]any{
			"server": serverURL,
			"token":  "openlist-token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	return storage
}
