package service

import (
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestApplyProviderMatchPersistsSeriesArtworkSeparatelyFromEpisodeStill(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{})
	repos := repository.New(db)
	library := model.Library{Name: "TV", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	const (
		seriesPoster   = "https://image.example/lucifer-poster.jpg"
		seriesBackdrop = "https://image.example/lucifer-series-backdrop.jpg"
		episodeStill   = "https://image.example/lucifer-s01e01-still.jpg"
		updatedStill   = "https://image.example/lucifer-s01e01-updated-still.jpg"
	)
	episode := model.Media{
		LibraryID:   library.ID,
		Title:       "路西法",
		Path:        "/media/tv/Lucifer/Season 01/Lucifer - S01E01.mkv",
		SeasonNum:   1,
		EpisodeNum:  1,
		BackdropURL: episodeStill,
	}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatal(err)
	}
	scraper := NewScraperService(
		&config.Config{}, zap.NewNop(), repos, nil, nil, nil, nil, NewHub(zap.NewNop()),
	)
	match := &Match{
		TMDbID:       63174,
		MediaType:    "tv",
		Title:        "路西法",
		OriginalName: "Lucifer",
		PosterURL:    seriesPoster,
		BackdropURL:  seriesBackdrop,
		Overview:     "Series overview",
		Year:         2016,
	}
	options := ScrapeOptions{DeferEpisodeDetails: true, deferTMDbDetails: true, deferPeople: true}
	if err := scraper.applyProviderMatchWithOptions(t.Context(), &episode, &library, match, options); err != nil {
		t.Fatal(err)
	}

	var storedEpisode model.Media
	if err := db.First(&storedEpisode, "id = ?", episode.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedEpisode.BackdropURL != episodeStill {
		t.Fatalf("episode backdrop = %q, want existing still %q", storedEpisode.BackdropURL, episodeStill)
	}
	if storedEpisode.SeriesID != "" {
		t.Fatalf("scrape changed public grouping identity through SeriesID = %q", storedEpisode.SeriesID)
	}

	var storedSeries model.Series
	if err := db.Where("library_id = ? AND tm_db_id = ?", library.ID, 63174).First(&storedSeries).Error; err != nil {
		t.Fatal(err)
	}
	if storedSeries.PosterURL != seriesPoster || storedSeries.BackdropURL != seriesBackdrop {
		t.Fatalf("series artwork = poster %q backdrop %q", storedSeries.PosterURL, storedSeries.BackdropURL)
	}
	if storedSeries.Title != "路西法" || storedSeries.Overview != "Series overview" || storedSeries.Year != 2016 {
		t.Fatalf("series metadata = %#v", storedSeries)
	}

	if saved, err := scraper.saveTMDbEpisodeDetailsResult(t.Context(), &storedEpisode, 63174, 2016, &TMDbEpisodeDetails{StillURL: updatedStill}); err != nil || !saved {
		t.Fatalf("save episode details: saved=%v err=%v", saved, err)
	}
	if err := db.First(&storedSeries, "id = ?", storedSeries.ID).Error; err != nil {
		t.Fatal(err)
	}
	if storedSeries.BackdropURL != seriesBackdrop {
		t.Fatalf("episode detail overwrote series backdrop: got %q want %q", storedSeries.BackdropURL, seriesBackdrop)
	}
}

func TestEmbySeriesUsesPersistedSeriesBackdropAndKeepsEpisodeStill(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Series{}, &model.Media{})
	repos := repository.New(db)
	library := model.Library{Name: "TV", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := db.Create(&library).Error; err != nil {
		t.Fatal(err)
	}
	const (
		seriesBackdrop = "https://image.example/lucifer-series-backdrop.jpg"
		episodeStill   = "https://image.example/lucifer-s01e01-still.jpg"
	)
	series := model.Series{
		LibraryID:   library.ID,
		Title:       "路西法",
		PosterURL:   "https://image.example/lucifer-poster.jpg",
		BackdropURL: seriesBackdrop,
		TMDbID:      63174,
	}
	if err := db.Create(&series).Error; err != nil {
		t.Fatal(err)
	}
	episode := model.Media{
		LibraryID:   library.ID,
		Title:       "路西法",
		Path:        "/media/tv/Lucifer/Season 01/Lucifer - S01E01.mkv",
		SeasonNum:   1,
		EpisodeNum:  1,
		TMDbID:      63174,
		BackdropURL: episodeStill,
	}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatal(err)
	}

	emby := NewEmbyService(&config.Config{}, zap.NewNop(), repos)
	groups, err := emby.seriesGroupsFromMedia(t.Context(), []model.Media{episode})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 {
		t.Fatalf("series groups = %d, want 1", len(groups))
	}
	if groups[0].BackdropURL != seriesBackdrop {
		t.Fatalf("series backdrop = %q, want %q", groups[0].BackdropURL, seriesBackdrop)
	}
	if groups[0].ID != stableEmbyID(embyVirtualSeriesPrefix, library.ID, "路西法") {
		t.Fatalf("series public id changed: %q", groups[0].ID)
	}
	payload := emby.seriesPayload(groups[0])
	backdropTags, ok := payload["BackdropImageTags"].([]string)
	if !ok || len(backdropTags) != 1 || backdropTags[0] != embyImageTag(groups[0].ID, "backdrop", seriesBackdrop, series.UpdatedAt) {
		t.Fatalf("series backdrop tags = %#v", payload["BackdropImageTags"])
	}
	if got, err := emby.ImageURL(t.Context(), groups[0].ID, "Backdrop"); err != nil || got != seriesBackdrop {
		t.Fatalf("series image = %q err=%v, want %q", got, err, seriesBackdrop)
	}
	if got, err := emby.ImageURL(t.Context(), episode.ID, "Primary"); err != nil || got != episodeStill {
		t.Fatalf("episode primary = %q err=%v, want %q", got, err, episodeStill)
	}
}

func TestScrapeInvalidationClearsVirtualSeriesArtwork(t *testing.T) {
	emby := NewEmbyService(&config.Config{}, zap.NewNop(), nil)
	emby.rememberSeriesGroup(embySeriesGroup{
		ID:          "msgo-series-cached",
		PosterURL:   "https://image.example/old-poster.jpg",
		BackdropURL: "https://image.example/old-backdrop.jpg",
	})
	scraper := (&ScraperService{}).SetMediaChangeHandler(emby.invalidateVirtualSeriesCache)
	scraper.invalidateMediaCache(t.Context())
	if _, ok := emby.cachedSeriesGroup("msgo-series-cached"); ok {
		t.Fatal("series cache survived scrape invalidation")
	}
	if _, ok := emby.cachedArtworkURL("msgo-series-cached", "Backdrop"); ok {
		t.Fatal("series artwork cache survived scrape invalidation")
	}
}
