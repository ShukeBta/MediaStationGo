package service

import (
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestManualEnrichLibraryRetriesNoMatchAndCountsRealMatches(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	mediaPath := filepath.Join(lib.Path, "间谍过家家 - S02E02.mkv")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   2,
		ScrapeStatus: "no_match",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if matched, err := scraper.EnrichLibrary(t.Context(), lib.ID); err != nil || matched != 0 {
		t.Fatalf("default EnrichLibrary matched=%d err=%v, want skipped no_match", matched, err)
	}
	if matched, err := scraper.EnrichLibrary(t.Context(), lib.ID, true); err != nil || matched != 1 {
		t.Fatalf("manual EnrichLibrary matched=%d err=%v, want one real match", matched, err)
	}
}

func TestManualEnrichLibraryCanRefreshAlreadyMatchedRows(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         filepath.Join(lib.Path, "间谍过家家 - S02E02.mkv"),
		SeasonNum:    2,
		EpisodeNum:   2,
		ScrapeStatus: "matched",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	defaultResult, err := scraper.EnrichLibraryDetailedWithOptions(t.Context(), lib.ID, ScrapeOptions{RetryNoMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	if defaultResult.Processed != 0 || defaultResult.Candidates != 0 {
		t.Fatalf("default manual scrape result=%+v, want matched rows skipped without IncludeMatched", defaultResult)
	}

	refreshResult, err := scraper.EnrichLibraryDetailedWithOptions(t.Context(), lib.ID, ScrapeOptions{
		RetryNoMatch:   true,
		IncludeMatched: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if refreshResult.Processed != 1 || refreshResult.Matched != 1 || refreshResult.Candidates != 1 {
		t.Fatalf("refresh result=%+v, want already matched row reprocessed", refreshResult)
	}
}

func TestScrapeCandidateRowsPrioritizeLibraryArtworkBeforeEpisodes(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	lib := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{
			Base:         model.Base{ID: "001-episode"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家 第 1 集",
			Path:         filepath.Join(lib.Path, "间谍过家家 - S02E01.mkv"),
			SeasonNum:    2,
			EpisodeNum:   1,
			ScrapeStatus: "pending",
		},
		{
			Base:         model.Base{ID: "999-series"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家",
			Path:         filepath.Join(lib.Path, "间谍过家家.mkv"),
			ScrapeStatus: "pending",
		},
	}
	if err := repos.DB.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	got, err := scraper.scrapeCandidateRows(t.Context(), lib.ID, ScrapeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("candidate rows = %d, want 2", len(got))
	}
	if got[0].ID != "999-series" || got[1].ID != "001-episode" {
		t.Fatalf("scrape order = [%s, %s], want series-level row before episode row", got[0].ID, got[1].ID)
	}
}

func TestEnrichLibraryIncludesMergedCloudLibraryMedia(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	local := model.Library{Name: "番剧", Path: t.TempDir(), Type: "tv", Enabled: true}
	cloud := model.Library{
		Name:    "OpenList · 番剧",
		Path:    BuildCloudLibraryPath("openlist", "/番剧", "/番剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.DB.Create(&local).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&cloud).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		LibraryID:    cloud.ID,
		Title:        "间谍过家家",
		Path:         "cloud://openlist/番剧/间谍过家家 - S02E02.mkv",
		SeasonNum:    2,
		EpisodeNum:   2,
		ScrapeStatus: "pending",
	}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := scraper.EnrichLibraryDetailed(t.Context(), local.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if result.Matched != 1 || result.Processed != 1 || result.Candidates != 1 || result.Failed != 0 {
		t.Fatalf("result=%+v, want merged cloud media to be scraped once", result)
	}
	var got model.Media
	if err := repos.DB.First(&got, "library_id = ?", cloud.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.TMDbID != 12345 {
		t.Fatalf("merged cloud media was not enriched: status=%q tmdb=%d", got.ScrapeStatus, got.TMDbID)
	}
}
