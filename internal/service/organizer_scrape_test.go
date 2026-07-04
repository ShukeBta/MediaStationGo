package service

import (
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectoryScanAndScrapeAfter(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()
	if err := repos.DB.AutoMigrate(&model.Setting{}); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	lib := model.Library{
		Name:    "剧集",
		Path:    filepath.Join(dest, "电视剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "tv",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1", res.Organized)
	}

	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, scraper)
	scans, scrapes := scanner.ScanAndScrapeLibrariesForPath(t.Context(), res.DestPath, "", true)
	if len(scans) != 1 || scans[0].Added != 1 {
		t.Fatalf("scans = %#v, want one scan with added=1", scans)
	}
	if len(scrapes) != 1 || scrapes[0].Matched != 1 || scrapes[0].Error != "" || scrapes[0].Skipped {
		t.Fatalf("scrapes = %#v, want one successful matched scrape", scrapes)
	}

	var media model.Media
	if err := repos.DB.Where("path LIKE ?", "%Spy Family - S01E01.mkv").First(&media).Error; err != nil {
		t.Fatal(err)
	}
	if media.ScrapeStatus != "matched" || media.TMDbID != 12345 {
		t.Fatalf("media scrape status=%q tmdb=%d, want matched/12345", media.ScrapeStatus, media.TMDbID)
	}
	if _, err := os.Stat(media.Path); err != nil {
		t.Fatalf("organized file missing at %q: %v", media.Path, err)
	}
}

func TestOrganizeDirectoryUsesScraperMatchBeforeRename(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: TransferCopy,
		MediaType:    "tv",
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	if res.Organized != 1 {
		t.Fatalf("organized = %d, want 1", res.Organized)
	}
	want := filepath.Join(dest, "电视剧", "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organized file should use matched metadata path %q: %v; items=%#v", want, err, res.Items)
	}
	if len(res.Items) != 1 || res.Items[0].Target != want || res.Items[0].Title != "间谍过家家" {
		t.Fatalf("organize preview did not use scraper metadata: %#v", res.Items)
	}
	var media model.Media
	if err := repos.DB.First(&media, "path = ?", want).Error; err != nil {
		t.Fatalf("organized metadata should be persisted before scan: %v", err)
	}
	if media.Title != "间谍过家家" || media.TMDbID != 12345 || media.ScrapeStatus != "matched" {
		t.Fatalf("persisted media = title=%q tmdb=%d status=%q, want localized matched metadata", media.Title, media.TMDbID, media.ScrapeStatus)
	}
}

func TestOrganizePipelineRenamesAfterScrape(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	lib := model.Library{
		Name:    "剧集",
		Path:    filepath.Join(dest, "电视剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, scraper)
	pipeline := NewOrganizePipelineService(zap.NewNop(), repos, organizer, scanner, nil)
	_, err := pipeline.Run(t.Context(), OrganizePipelineRequest{
		Scope:        OrganizeScopeDirectory,
		Trigger:      OrganizeTriggerManual,
		SourcePath:   src,
		DestPath:     dest,
		TransferMode: string(TransferCopy),
		MediaType:    "tv",
	})
	if err != nil {
		t.Fatalf("pipeline organize: %v", err)
	}

	englishPath := filepath.Join(dest, "电视剧", "Spy Family", "Season 01", "Spy Family - S01E01.mkv")
	if _, err := os.Stat(englishPath); !os.IsNotExist(err) {
		t.Fatalf("english pre-scrape path should be renamed away, stat err=%v", err)
	}
	englishDir := filepath.Join(dest, "电视剧", "Spy Family")
	if _, err := os.Stat(englishDir); !os.IsNotExist(err) {
		t.Fatalf("empty english pre-scrape directory should be cleaned up, stat err=%v", err)
	}
	want := filepath.Join(dest, "电视剧", "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("scraped rename target missing %q: %v", want, err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "间谍过家家" || got.ReleaseDate != "2022-04-09" || got.ScrapeStatus != "matched" {
		t.Fatalf("media after scrape rename = title=%q release=%q status=%q", got.Title, got.ReleaseDate, got.ScrapeStatus)
	}
}

func TestOrganizeMediaRefreshesMetadataBeforeRename(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media", "电视剧")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	lib := model.Library{
		Name:    "剧集",
		Path:    dest,
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Spy x Family S01E01 2022 1080p",
		Path:         sourceFile,
		Container:    "mkv",
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "pending",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	dst, err := organizer.OrganizeMediaWithOptions(t.Context(), media.ID, OrganizeOptions{
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize media: %v", err)
	}
	want := filepath.Join(dest, "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "间谍过家家" || got.TMDbID != 12345 || got.ScrapeStatus != "matched" {
		t.Fatalf("media = title=%q tmdb=%d status=%q, want localized matched metadata", got.Title, got.TMDbID, got.ScrapeStatus)
	}
	if got.Path != want {
		t.Fatalf("media path = %q, want %q", got.Path, want)
	}
}

func TestOrganizeMediaRefreshesMatchedReleaseTitleBeforeRename(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media", "电视剧")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.1080p.WEB-DL.mkv")
	writeOrgFile(t, sourceFile, "episode")

	lib := model.Library{Name: "剧集", Path: dest, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Spy.x.Family.S01E01.2022.1080p.WEB-DL",
		Path:         sourceFile,
		Container:    "mkv",
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       12345,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	dst, err := organizer.OrganizeMediaWithOptions(t.Context(), media.ID, OrganizeOptions{
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize media: %v", err)
	}
	want := filepath.Join(dest, "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	if dst != want {
		t.Fatalf("dst = %q, want %q", dst, want)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "间谍过家家" || got.TMDbID != 12345 || got.OriginalName != "SPY×FAMILY" {
		t.Fatalf("media = title=%q original=%q tmdb=%d, want refreshed localized metadata", got.Title, got.OriginalName, got.TMDbID)
	}
}

func TestOrganizeMediaPersistsMetadataWhenAlreadyInPlace(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	libRoot := filepath.Join(root, "media", "电视剧")
	mediaPath := filepath.Join(libRoot, "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	writeOrgFile(t, mediaPath, "episode")

	lib := model.Library{Name: "剧集", Path: libRoot, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Spy.x.Family.S01E01.2022.1080p.WEB-DL",
		Path:         mediaPath,
		Container:    "mkv",
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	dst, err := organizer.OrganizeMediaWithOptions(t.Context(), media.ID, OrganizeOptions{
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize media already in place: %v", err)
	}
	if dst != mediaPath {
		t.Fatalf("dst = %q, want existing path %q", dst, mediaPath)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "间谍过家家" || got.TMDbID != 12345 || got.OriginalName != "SPY×FAMILY" || got.ScrapeStatus != "matched" {
		t.Fatalf("metadata not persisted for already-in-place media: title=%q original=%q tmdb=%d status=%q", got.Title, got.OriginalName, got.TMDbID, got.ScrapeStatus)
	}
}

func TestOrganizeLibraryPersistsMetadataForInPlaceWeakRows(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	libRoot := filepath.Join(root, "media", "电视剧", "欧美剧")
	mediaPath := filepath.Join(libRoot, "间谍过家家", "Season 01", "间谍过家家 - S01E01.mkv")
	writeOrgFile(t, mediaPath, "episode")

	lib := model.Library{Name: "欧美剧", Path: libRoot, Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Spy.x.Family.S01E01.2022.1080p.WEB-DL",
		Path:         mediaPath,
		Container:    "mkv",
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(&config.Config{}, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeLibraryWithOptions(t.Context(), lib.ID, OrganizeOptions{})
	if err != nil {
		t.Fatalf("organize library: %v", err)
	}
	if res.Organized != 0 || res.Reclassified != 0 {
		t.Fatalf("result = %+v, want metadata-only refresh without move", res)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "间谍过家家" || got.TMDbID != 12345 || got.OriginalName != "SPY×FAMILY" || got.ScrapeStatus != "matched" {
		t.Fatalf("metadata not persisted for in-place library row: title=%q original=%q tmdb=%d status=%q", got.Title, got.OriginalName, got.TMDbID, got.ScrapeStatus)
	}
}

func TestOrganizeScanAndScrapeRetriesNoMatchRows(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	libRoot := filepath.Join(root, "media", "电视剧")
	mediaPath := filepath.Join(libRoot, "间谍过家家", "Season 02", "间谍过家家 - S02E02.mkv")
	writeOrgFile(t, mediaPath, "episode")

	lib := model.Library{
		Name:    "剧集",
		Path:    libRoot,
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "间谍过家家",
		Path:         mediaPath,
		SeasonNum:    2,
		EpisodeNum:   2,
		ScrapeStatus: "no_match",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, scraper)
	_, scrapes := scanner.ScanAndScrapeLibrariesForPath(t.Context(), filepath.Join(root, "media"), "", true)
	if len(scrapes) != 1 || scrapes[0].Matched != 1 || scrapes[0].Error != "" || scrapes[0].Skipped {
		t.Fatalf("scrapes = %#v, want one retried no_match row", scrapes)
	}

	var got model.Media
	if err := repos.DB.First(&got, "path = ?", mediaPath).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.TMDbID != 12345 {
		t.Fatalf("media scrape status=%q tmdb=%d, want matched/12345", got.ScrapeStatus, got.TMDbID)
	}
}

func TestOrganizeScanAndScrapeRepairsWeakMatchedReleaseTitle(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	libRoot := filepath.Join(root, "media", "电视剧")
	mediaPath := filepath.Join(libRoot, "Spy.x.Family", "Season 01", "Spy.x.Family.S01E01.2022.1080p.WEB-DL.mkv")
	writeOrgFile(t, mediaPath, "episode")

	lib := model.Library{
		Name:    "剧集",
		Path:    libRoot,
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "Spy.x.Family.S01E01.2022.1080p.WEB-DL",
		Path:         mediaPath,
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "matched",
	}
	if err := repos.Media.Upsert(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	scanner := NewScannerService(&config.Config{}, zap.NewNop(), repos, NewHub(zap.NewNop()), nil, scraper)
	_, scrapes := scanner.ScanAndScrapeLibrariesForPath(t.Context(), libRoot, "", true)
	if len(scrapes) != 1 || scrapes[0].Matched != 1 || scrapes[0].Processed != 1 || scrapes[0].Error != "" || scrapes[0].Skipped {
		t.Fatalf("scrapes = %#v, want one repaired matched release row", scrapes)
	}

	var got model.Media
	if err := repos.DB.First(&got, "path = ?", mediaPath).Error; err != nil {
		t.Fatal(err)
	}
	if got.Title != "间谍过家家" || got.TMDbID != 12345 || got.ScrapeStatus != "matched" {
		t.Fatalf("media = title=%q tmdb=%d status=%q, want localized matched metadata", got.Title, got.TMDbID, got.ScrapeStatus)
	}
}

func TestOrganizeResultNeedsVisibilitySyncIgnoresScannedDuplicates(t *testing.T) {
	if OrganizeResultNeedsVisibilitySync(&OrganizeResult{
		Skipped: 1,
		Items:   []OrganizePreviewItem{{Action: "skip", Reason: organizeSkipDuplicateLibrary}},
	}) {
		t.Fatal("already-scanned duplicate should not trigger another visibility scan")
	}
	if OrganizeResultNeedsVisibilitySync(&OrganizeResult{
		Skipped: 1,
		Items:   []OrganizePreviewItem{{Action: "skip", Reason: organizeSkipSampleClip}},
	}) {
		t.Fatal("sample clip skip should not trigger visibility scan")
	}
	if !OrganizeResultNeedsVisibilitySync(&OrganizeResult{
		Skipped: 1,
		Items:   []OrganizePreviewItem{{Action: "skip", Reason: organizeSkipTargetExists}},
	}) {
		t.Fatal("unscanned target file should trigger visibility scan")
	}
	if !OrganizeResultNeedsVisibilitySync(&OrganizeResult{Organized: 1}) {
		t.Fatal("organized files must trigger visibility scan")
	}
	if !OrganizeResultNeedsVisibilitySync(&OrganizeResult{Reclassified: 1}) {
		t.Fatal("reclassified files must trigger visibility scan")
	}
}

func TestOrganizeScrapeAfterEnabledDefaultsOn(t *testing.T) {
	if !OrganizeScrapeAfterEnabled(t.Context(), nil) {
		t.Fatalf("organize scrape-after should default on without a repo")
	}
	repos := newOrganizerTestRepo(t)
	if !OrganizeScrapeAfterEnabled(t.Context(), repos) {
		t.Fatalf("organize scrape-after should default on when setting is absent")
	}
	if err := repos.Setting.Set(t.Context(), "organize.scrape_after", "false"); err != nil {
		t.Fatal(err)
	}
	if OrganizeScrapeAfterEnabled(t.Context(), repos) {
		t.Fatalf("explicit organize.scrape_after=false should be respected")
	}
}
