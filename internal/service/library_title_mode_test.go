package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestPreserveSourceTitleIdentityKeepsOriginalFilename(t *testing.T) {
	media := &model.Media{
		Title:        "cleaned title",
		Year:         2025,
		SeasonNum:    2,
		EpisodeNum:   7,
		SeriesID:     "series:test",
		EpisodeTitle: "第七集",
	}

	preserveSourceTitleIdentity(media, "【原始发布组】作品.Name_01 [1080P].mkv")

	if media.Title != "【原始发布组】作品.Name_01 [1080P]" {
		t.Fatalf("title = %q", media.Title)
	}
	if media.Year != 0 || media.SeasonNum != 0 || media.EpisodeNum != 0 || media.SeriesID != "" || media.EpisodeTitle != "" {
		t.Fatalf("identity was not flattened: %+v", media)
	}
	if !media.PreserveSourceTitle {
		t.Fatal("preserve source title marker is false")
	}
}

func TestNormalizeLibraryTitleMode(t *testing.T) {
	for input, want := range map[string]string{
		"":         LibraryTitleModeSmart,
		"SMART":    LibraryTitleModeSmart,
		"filename": LibraryTitleModeFilename,
	} {
		got, err := NormalizeLibraryTitleMode(input)
		if err != nil || got != want {
			t.Fatalf("NormalizeLibraryTitleMode(%q) = %q, %v", input, got, err)
		}
	}
	if _, err := NormalizeLibraryTitleMode("unknown"); err == nil {
		t.Fatal("invalid title mode should fail")
	}
}

func TestBuildLocalScanMediaUsesFilenameTitleMode(t *testing.T) {
	scanner := &ScannerService{}
	media := scanner.buildLocalScanMedia(localScanMediaInput{
		lib:           &model.Library{TitleMode: LibraryTitleModeFilename},
		path:          `/media/其他/【原始发布组】作品.Name_01 [1080P].mkv`,
		ext:           ".mkv",
		parsedSeason:  1,
		parsedEpisode: 8,
	})

	if media.Title != "【原始发布组】作品.Name_01 [1080P]" || media.SeasonNum != 0 || media.EpisodeNum != 0 {
		t.Fatalf("media identity = %+v", media)
	}
}

func TestLibraryAllowsAutoScrapeRejectsFilenameMode(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{})
	repos := repository.New(db)
	filenameLib := model.Library{Name: "其他媒体", Path: "/media/other", Type: "movie", TitleMode: LibraryTitleModeFilename, Enabled: true}
	smartLib := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", TitleMode: LibraryTitleModeSmart, Enabled: true}
	for _, lib := range []*model.Library{&filenameLib, &smartLib} {
		if err := repos.DB.Create(lib).Error; err != nil {
			t.Fatal(err)
		}
	}
	scanner := &ScannerService{repo: repos}
	if scanner.libraryAllowsAutoScrape(t.Context(), filenameLib.ID) {
		t.Fatal("filename title mode must skip automatic scraping")
	}
	if !scanner.libraryAllowsAutoScrape(t.Context(), smartLib.ID) {
		t.Fatal("smart title mode should allow automatic scraping")
	}
}
