package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectoryReclassifiesScannedAnimeUsingDBMetadata(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	tvAnimeLib := model.Library{Name: "国漫", Path: filepath.Join(dest, "电视剧", "国漫"), Type: "tv", Enabled: true}
	animeLib := model.Library{Name: "国漫", Path: filepath.Join(dest, "动漫", "国漫"), Type: "anime", Enabled: true}
	for _, lib := range []*model.Library{&euusLib, &tvAnimeLib, &animeLib} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}

	wrongPath := filepath.Join(euusLib.Path, "Blades Of The Guardians", "Season 2", "Blades Of The Guardians - S02E01-1080p.TX.WEB-DL.mkv")
	writeOrgFile(t, wrongPath, "episode")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    euusLib.ID,
		Title:        "镖人",
		OriginalName: "Blades Of The Guardians",
		Path:         wrongPath,
		SeasonNum:    2,
		EpisodeNum:   1,
		TMDbID:       107463,
		Languages:    "zh",
		Countries:    "CN",
		Genres:       "动画,动作冒险",
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   filepath.Join(euusLib.Path, "Blades Of The Guardians"),
		DestPath:     euusLib.Path,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(animeLib.Path, "镖人", "Season 02", "镖人 - S02E01.mkv")
	if res.Reclassified != 1 || res.Organized != 0 {
		t.Fatalf("result = %+v, want scanned DB metadata reclassified only", res)
	}
	if _, err := os.Stat(wrongPath); !os.IsNotExist(err) {
		t.Fatalf("wrong anime path should be moved away, stat err=%v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("anime should move to physical anime library at %q: %v; items=%#v", want, err, res.Items)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != animeLib.ID {
		t.Fatalf("library_id = %q, want anime library %q", got.LibraryID, animeLib.ID)
	}
}

func TestReclassifyMisclassifiedMediaMovesScannedAnimeToPhysicalAnimeLibrary(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	tvAnimeLib := model.Library{Name: "国漫", Path: filepath.Join(dest, "电视剧", "国漫"), Type: "tv", Enabled: true}
	animeLib := model.Library{Name: "国漫", Path: filepath.Join(dest, "动漫", "国漫"), Type: "anime", Enabled: true}
	for _, lib := range []*model.Library{&euusLib, &tvAnimeLib, &animeLib} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}

	wrongPath := filepath.Join(euusLib.Path, "Blades Of The Guardians", "Season 2", "Blades Of The Guardians - S02E01.mkv")
	writeOrgFile(t, wrongPath, "episode")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    euusLib.ID,
		Title:        "镖人",
		OriginalName: "Blades Of The Guardians",
		Path:         wrongPath,
		SeasonNum:    2,
		EpisodeNum:   1,
		TMDbID:       107463,
		Languages:    "zh",
		Countries:    "CN",
		Genres:       "动画,动作冒险",
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	res, err := organizer.ReclassifyMisclassifiedMedia(t.Context(), MediaCategoryReclassifyOptions{})
	if err != nil {
		t.Fatalf("reclassify media: %v", err)
	}
	want := filepath.Join(animeLib.Path, "镖人", "Season 02", "镖人 - S02E01.mkv")
	if res.Reclassified != 1 {
		t.Fatalf("reclassified = %d, want 1; items=%#v errors=%#v", res.Reclassified, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("bulk reclassify target missing at %q: %v", want, err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != animeLib.ID {
		t.Fatalf("library_id = %q, want anime library %q", got.LibraryID, animeLib.ID)
	}
}

func TestReclassifyMisclassifiedMediaCreatesMissingTargetCategoryLibrary(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &euusLib); err != nil {
		t.Fatal(err)
	}

	wrongPath := filepath.Join(euusLib.Path, "Blades Of The Guardians", "Season 2", "Blades Of The Guardians - S02E01.mkv")
	writeOrgFile(t, wrongPath, "episode")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    euusLib.ID,
		Title:        "镖人",
		OriginalName: "Blades Of The Guardians",
		Path:         wrongPath,
		SeasonNum:    2,
		EpisodeNum:   1,
		TMDbID:       107463,
		Languages:    "zh",
		Countries:    "CN",
		Genres:       "动画,动作冒险",
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	res, err := organizer.ReclassifyMisclassifiedMedia(t.Context(), MediaCategoryReclassifyOptions{})
	if err != nil {
		t.Fatalf("reclassify media: %v", err)
	}

	targetRoot := filepath.Join(dest, "动漫", "国漫")
	want := filepath.Join(targetRoot, "镖人", "Season 02", "镖人 - S02E01.mkv")
	if res.Reclassified != 1 {
		t.Fatalf("reclassified = %d, want 1; items=%#v errors=%#v", res.Reclassified, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("created category target missing at %q: %v", want, err)
	}

	var created model.Library
	if err := repos.DB.Where("path = ?", targetRoot).First(&created).Error; err != nil {
		t.Fatalf("missing auto-created anime library: %v", err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != created.ID {
		t.Fatalf("library_id = %q, want auto-created library %q", got.LibraryID, created.ID)
	}
}

func TestReclassifyMisclassifiedMediaMovesWesternAnimationToWesternAnimeLibrary(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	jpAnimeLib := model.Library{Name: "日番", Path: filepath.Join(dest, "动漫", "日番"), Type: "anime", Enabled: true}
	westernAnimeLib := model.Library{Name: "美漫", Path: filepath.Join(dest, "动漫", "美漫"), Type: "anime", Enabled: true}
	for _, lib := range []*model.Library{&jpAnimeLib, &westernAnimeLib} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}

	wrongPath := filepath.Join(jpAnimeLib.Path, "Family Guy", "Season 10", "Family Guy - S10E15.mkv")
	writeOrgFile(t, wrongPath, "episode")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    jpAnimeLib.ID,
		Title:        "恶搞之家",
		OriginalName: "Family Guy",
		Path:         wrongPath,
		SeasonNum:    10,
		EpisodeNum:   15,
		TMDbID:       1434,
		Languages:    "en",
		Countries:    "US",
		Genres:       "动画,喜剧",
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	res, err := organizer.ReclassifyMisclassifiedMedia(t.Context(), MediaCategoryReclassifyOptions{})
	if err != nil {
		t.Fatalf("reclassify media: %v", err)
	}
	want := filepath.Join(westernAnimeLib.Path, "恶搞之家", "Season 10", "恶搞之家 - S10E15.mkv")
	if res.Reclassified != 1 {
		t.Fatalf("reclassified = %d, want 1; items=%#v errors=%#v", res.Reclassified, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("western animation target missing at %q: %v", want, err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != westernAnimeLib.ID {
		t.Fatalf("library_id = %q, want western anime library %q", got.LibraryID, westernAnimeLib.ID)
	}
}
