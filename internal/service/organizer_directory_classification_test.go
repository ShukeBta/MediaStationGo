package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectoryUsesDownloadCategoryLayout(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "国产剧", "狂飙.S01E01.2023.1080p.WEB-DL.mkv"), "kuangbiao-e01")
	writeOrgFile(t, filepath.Join(src, "华语电影", "流浪地球2.2023.2160p.WEB-DL.H265.mkv"), "wandering-earth-2")

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), newOrganizerTestRepo(t))
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 2 || res.Replaced != 0 || res.Skipped != 0 {
		t.Fatalf("expected organized=2 replaced=0 skipped=0, got %+v", res)
	}

	tv := filepath.Join(dest, "电视剧", "国产剧", "狂飙", "Season 01", "狂飙 - S01E01.mkv")
	if _, err := os.Stat(tv); err != nil {
		t.Fatalf("expected TV episode organized at %q: %v", tv, err)
	}
	movie := filepath.Join(dest, "电影", "华语电影", "流浪地球2 (2023)", "流浪地球2 (2023).mkv")
	if _, err := os.Stat(movie); err != nil {
		t.Fatalf("expected movie organized at %q: %v", movie, err)
	}
}

func TestOrganizeDirectoryUsesExplicitCategoryLibraryRoot(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Motherhood.of.Taihang.S01E01.2026.1080p.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "episode")

	repos := newOrganizerTestRepo(t)
	libraryRoot := filepath.Join(dest, "电视剧", "国产剧")
	wrongType := model.Library{Name: "国产剧", Path: libraryRoot, Type: "movie", Enabled: true}
	rightType := model.Library{Name: "国产剧", Path: libraryRoot, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &wrongType); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &rightType); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:    src,
		DestPath:      dest,
		MediaType:     "tv",
		MediaCategory: "国产剧",
		TransferMode:  TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize explicit category: %v", err)
	}
	if res.Organized != 1 || len(res.Items) != 1 {
		t.Fatalf("result = %+v, want one organized item", res)
	}
	if !pathWithin(res.Items[0].Target, libraryRoot) {
		t.Fatalf("target = %q, want under %q", res.Items[0].Target, libraryRoot)
	}
	if pathWithin(res.Items[0].Target, filepath.Join(dest, "电视剧")) && !pathWithin(res.Items[0].Target, libraryRoot) {
		t.Fatalf("target landed outside category library: %q", res.Items[0].Target)
	}
}

func TestOrganizeDirectoryDoesNotTargetLegacyCategoryLibrary(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Some.Movie.2026.1080p.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "movie")

	repos := newOrganizerTestRepo(t)
	legacyRoot := filepath.Join(dest, "电影", "外语电影")
	currentRoot := filepath.Join(dest, "电影", "欧美电影")
	legacyLib := model.Library{Name: "外语电影", Path: legacyRoot, Type: "movie", Enabled: true}
	currentLib := model.Library{Name: "欧美电影", Path: currentRoot, Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &legacyLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &currentLib); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:    src,
		DestPath:      dest,
		MediaType:     "movie",
		MediaCategory: "外语电影",
		TransferMode:  TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize legacy category: %v", err)
	}
	if res.Organized != 1 || len(res.Items) != 1 {
		t.Fatalf("result = %+v, want one organized item", res)
	}
	if !pathWithin(res.Items[0].Target, currentRoot) {
		t.Fatalf("target = %q, want current category root %q", res.Items[0].Target, currentRoot)
	}
	if pathWithin(res.Items[0].Target, legacyRoot) {
		t.Fatalf("target must not use legacy category root %q", res.Items[0].Target)
	}
}

func TestOrganizeDirectoryTreatsCategoryDestAsCollectionRoot(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Some.Show.S01E01.2026.1080p.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "episode")

	repos := newOrganizerTestRepo(t)
	currentLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &currentLib); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:    src,
		DestPath:      currentLib.Path,
		MediaType:     "tv",
		MediaCategory: "国产剧",
		TransferMode:  TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize category-root dest: %v", err)
	}
	if res.Organized != 1 || len(res.Items) != 1 {
		t.Fatalf("result = %+v, want one organized item", res)
	}

	wantRoot := filepath.Join(dest, "电视剧", "国产剧")
	want := filepath.Join(wantRoot, "Some Show", "Season 01", "Some Show - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected sibling category target at %q: %v; items=%#v", want, err, res.Items)
	}
	nested := filepath.Join(currentLib.Path, "国产剧", "Some Show", "Season 01", "Some Show - S01E01.mkv")
	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("must not nest corrected category under current library, stat err=%v", err)
	}

	var created model.Library
	if err := repos.DB.Where("path = ?", wantRoot).First(&created).Error; err != nil {
		t.Fatalf("corrected category library should be auto-created: %v", err)
	}
}

func TestOrganizeDirectoryCreatesAnimeCategoryUnderPhysicalAnimeRoot(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads", "Blades.of.the.Guardians.S02E01.2026.1080p.mkv")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, src, "episode")

	repos := newOrganizerTestRepo(t)
	oldWrongLib := model.Library{Name: "国漫", Path: filepath.Join(dest, "电视剧", "国漫"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &oldWrongLib); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:    src,
		DestPath:      oldWrongLib.Path,
		MediaType:     "anime",
		MediaCategory: "国漫",
		TransferMode:  TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize anime category: %v", err)
	}
	wantRoot := filepath.Join(dest, "动漫", "国漫")
	want := filepath.Join(wantRoot, "Blades Of The Guardians", "Season 02", "Blades Of The Guardians - S02E01.mkv")
	if res.Organized != 1 || len(res.Items) != 1 {
		t.Fatalf("result = %+v, want one organized anime item", res)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("anime category should be created under physical anime root %q: %v; items=%#v", want, err, res.Items)
	}
	wrong := filepath.Join(oldWrongLib.Path, "Blades Of The Guardians", "Season 02", "Blades Of The Guardians - S02E01.mkv")
	if _, err := os.Stat(wrong); !os.IsNotExist(err) {
		t.Fatalf("must not keep anime category under old TV root %q, stat err=%v", wrong, err)
	}
	var created model.Library
	if err := repos.DB.Where("path = ?", wantRoot).First(&created).Error; err != nil {
		t.Fatalf("missing physical anime category library: %v", err)
	}
	if created.Type != "tv" || created.Name != "国漫" {
		t.Fatalf("created library = %+v, want visible tv-compatible 国漫 library", created)
	}
}

func TestOrganizeDirectoryCreatesMissingCategoryLibraryForVisibility(t *testing.T) {
	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	source := filepath.Join(srcRoot, "Gourd.Brothers.S01E01.2026.1080p.mkv")
	target := filepath.Join(dest, "电视剧", "欧美剧", "Gourd Brothers", "Season 01", "Gourd Brothers - S01E01.mkv")
	writeOrgFile(t, source, "source")
	writeOrgFile(t, target, "already-there")

	repos := newOrganizerTestRepo(t)
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:           srcRoot,
		DestPath:             dest,
		MediaType:            "tv",
		MediaCategory:        "欧美剧",
		TransferMode:         TransferCopy,
		AllowReplaceExisting: false,
	})
	if err != nil {
		t.Fatalf("organize missing category: %v", err)
	}
	if res.Organized != 0 || res.Skipped != 1 || len(res.Items) != 1 || res.Items[0].Reason != organizeSkipTargetExists {
		t.Fatalf("result = %+v, want skipped target exists", res)
	}

	var lib model.Library
	if err := repos.DB.Where("path = ?", filepath.Join(dest, "电视剧", "欧美剧")).First(&lib).Error; err != nil {
		t.Fatalf("missing auto-created category library: %v", err)
	}
	if lib.Name != "欧美剧" || lib.Type != "tv" || !lib.Enabled {
		t.Fatalf("auto-created library = %+v, want enabled tv 欧美剧", lib)
	}

	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	scans := scanner.ScanLibrariesForPath(t.Context(), res.DestPath, "")
	added := 0
	for _, scan := range scans {
		if scan.Error != "" {
			t.Fatalf("scan failed: %#v", scan)
		}
		added += scan.Added
	}
	if added != 1 {
		t.Fatalf("scan added = %d, want 1; scans=%#v", added, scans)
	}
}

func TestOrganizeDirectoryCanDisableAutoAddLibrary(t *testing.T) {
	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	source := filepath.Join(srcRoot, "Some.Show.S01E01.2026.1080p.mkv")
	writeOrgFile(t, source, "episode")

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organize.auto_add_library", "false"); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:    srcRoot,
		DestPath:      dest,
		MediaType:     "tv",
		MediaCategory: "欧美剧",
		TransferMode:  TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize missing category with auto-add disabled: %v", err)
	}
	if res.Organized != 1 || len(res.Items) != 1 {
		t.Fatalf("result = %+v, want one organized item", res)
	}
	want := filepath.Join(dest, "电视剧", "欧美剧", "Some Show", "Season 01", "Some Show - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized file at %q: %v", want, err)
	}

	var count int64
	if err := repos.DB.Model(&model.Library{}).Where("path = ?", filepath.Join(dest, "电视剧", "欧美剧")).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("auto-add disabled should not create library, got %d rows", count)
	}
}

func TestOrganizeDirectorySmartClassifiesUncategorizedSources(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "流浪地球2.2023.2160p.WEB-DL.mkv"), "cn-movie")
	writeOrgFile(t, filepath.Join(src, "Dune.2021.2160p.WEB-DL.mkv"), "foreign-movie")
	writeOrgFile(t, filepath.Join(src, "狂飙.S01E01.2023.1080p.WEB-DL.mkv"), "cn-tv")
	writeOrgFile(t, filepath.Join(src, "The.Last.of.Us.S01E01.2023.1080p.WEB-DL.mkv"), "western-tv")

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organizer.smart_classify", "true"); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 4 {
		t.Fatalf("organized = %d, want 4; result=%+v", res.Organized, res)
	}

	for _, want := range []string{
		filepath.Join(dest, "电影", "华语电影", "流浪地球2 (2023)", "流浪地球2 (2023).mkv"),
		filepath.Join(dest, "电影", "欧美电影", "Dune (2021)", "Dune (2021).mkv"),
		filepath.Join(dest, "电视剧", "国产剧", "狂飙", "Season 01", "狂飙 - S01E01.mkv"),
		filepath.Join(dest, "电视剧", "未分类", "The Last Of Us", "Season 01", "The Last Of Us - S01E01.mkv"),
	} {
		if _, err := os.Stat(want); err != nil {
			t.Fatalf("expected smart classified file at %q: %v; items=%+v", want, err, res.Items)
		}
	}
}

func TestOrganizeDirectorySmartClassifiesWithLocalNFO(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "Some.Show.S01E01.2024.1080p.mkv"), "jp-anime")
	writeOrgFile(t, filepath.Join(src, "tvshow.nfo"), `<tvshow>
  <title>Some Show</title>
  <genre>Animation</genre>
  <country>JP</country>
  <language>ja</language>
</tvshow>`)

	repos := newOrganizerTestRepo(t)
	if err := repos.Setting.Set(t.Context(), "organizer.smart_classify", "true"); err != nil {
		t.Fatal(err)
	}
	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1", res.Organized)
	}
	want := filepath.Join(dest, "动漫", "日番", "Some Show", "Season 01", "Some Show - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected NFO classified episode at %q: %v", want, err)
	}
}

func TestOrganizeDirectoryScanAfterRecursesNestedDownloadFolders(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	writeOrgFile(t, filepath.Join(src, "国产剧", "子目录", "狂飙.S01E01.2023.1080p.WEB-DL.mkv"), "kuangbiao-e01")
	writeOrgFile(t, filepath.Join(src, "华语电影", "更深", "流浪地球2.2023.2160p.WEB-DL.H265.mkv"), "wandering-earth-2")

	repos := newOrganizerTestRepo(t)
	tvLib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	movieLib := model.Library{Name: "华语电影", Path: filepath.Join(dest, "电影", "华语电影"), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &tvLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &movieLib); err != nil {
		t.Fatal(err)
	}

	org := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := org.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 2 {
		t.Fatalf("organized = %d, want 2", res.Organized)
	}

	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, nil)
	scans := scanner.ScanLibrariesForPath(t.Context(), res.DestPath, "")
	if len(scans) != 2 {
		t.Fatalf("scans = %#v, want two matching libraries", scans)
	}
	added := 0
	for _, scan := range scans {
		if scan.Error != "" {
			t.Fatalf("scan failed: %#v", scan)
		}
		added += scan.Added
	}
	if added != 2 {
		t.Fatalf("scan added = %d, want 2", added)
	}
	var count int64
	if err := repos.DB.Model(&model.Media{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("media rows = %d, want 2", count)
	}
}

func TestSelectOrganizeScanTargetsDedupesSamePathByPathType(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "media", "电视剧", "国产剧")
	libraries := []model.Library{
		{Name: "国产剧", Path: path, Type: "movie", Enabled: true},
		{Name: "国产剧", Path: path, Type: "tv", Enabled: true},
	}

	targets := selectOrganizeScanTargets(libraries, filepath.Join(root, "media"), "")
	if len(targets) != 1 {
		t.Fatalf("targets = %#v, want one deduped target", targets)
	}
	if targets[0].Type != "tv" {
		t.Fatalf("target type = %q, want tv", targets[0].Type)
	}
}
