package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestScrapeQueryCandidatesUseMovieFolderForGenericFilename(t *testing.T) {
	lib := &model.Library{
		Path: `/media/movies`,
		Type: "movie",
	}
	media := &model.Media{
		Title: "00000",
		Path:  `/media/movies/Inception (2010)/BDMV/STREAM/00000.m2ts`,
	}

	got := scrapeQueryCandidates(media, lib)
	if len(got) == 0 {
		t.Fatal("scrapeQueryCandidates returned no candidates")
	}
	if got[0] != "inception" {
		t.Fatalf("first query candidate = %q, want movie folder title; all candidates=%#v", got[0], got)
	}
	for _, candidate := range got {
		switch strings.ToLower(candidate) {
		case "bdmv", "stream":
			t.Fatalf("query candidates kept technical filename/folder: %#v", got)
		}
	}
}

func TestScrapeQueryCandidatesUseMovieLibraryRootWhenMountedAtMovieFolder(t *testing.T) {
	lib := &model.Library{
		Path: `/media/movies/Inception (2010)`,
		Type: "movie",
	}
	media := &model.Media{
		Title: "00000",
		Path:  `/media/movies/Inception (2010)/BDMV/STREAM/00000.m2ts`,
	}

	got := scrapeQueryCandidates(media, lib)
	if len(got) == 0 {
		t.Fatal("scrapeQueryCandidates returned no candidates")
	}
	if got[0] != "inception" {
		t.Fatalf("first query candidate = %q, want movie library root title; all candidates=%#v", got[0], got)
	}
}

func TestScrapeQueryCandidatesDoNotUseMovieCollectionFolderAsTitle(t *testing.T) {
	lib := &model.Library{Path: `/media/movies`, Type: "movie"}
	media := &model.Media{
		Title: "The Hunger Games Catching Fire",
		Year:  2013,
		Path: `/media/movies/The.Hunger.Games.Complete.4-Film.Collection/` +
			`The.Hunger.Games.Catching.Fire.2013.2160p.mkv`,
	}

	got := scrapeQueryCandidates(media, lib)
	if len(got) == 0 {
		t.Fatal("scrapeQueryCandidates returned no candidates")
	}
	if got[0] != "the hunger games catching fire" {
		t.Fatalf("first query candidate = %q, want individual movie title; all candidates=%#v", got[0], got)
	}
	for _, candidate := range got {
		if strings.Contains(strings.ToLower(candidate), "complete 4 film collection") {
			t.Fatalf("movie collection folder leaked into scrape candidates: %#v", got)
		}
	}
}

func TestEnrichOneUsesMovieFolderWhenFilenameIsGeneric(t *testing.T) {
	var queries []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/movie" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("query") != "inception" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id":             27205,
				"title":          "Inception",
				"overview":       "A thief enters dreams.",
				"poster_path":    "/inception.jpg",
				"release_date":   "2010-07-16",
				"vote_average":   8.4,
				"original_title": "Inception",
			}},
		})
	}))
	defer upstream.Close()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	log := zap.NewNop()
	scraper := NewScraperService(cfg, log, repos, NewTMDbProvider(cfg, log, nil), nil, nil, nil, NewHub(log))

	lib := model.Library{Name: "Movies", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{
		LibraryID:    lib.ID,
		Title:        "00000",
		Path:         `/media/movies/Inception (2010)/BDMV/STREAM/00000.m2ts`,
		ScrapeStatus: "pending",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	if err := scraper.EnrichOne(t.Context(), &media); err != nil {
		t.Fatal(err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.ScrapeStatus != "matched" || got.TMDbID != 27205 || got.Title != "Inception" {
		t.Fatalf("generic filename scrape did not use folder title: status=%q tmdb=%d title=%q queries=%v", got.ScrapeStatus, got.TMDbID, got.Title, queries)
	}
	if len(queries) == 0 || queries[0] != "inception" {
		t.Fatalf("first tmdb query = %q, want folder title; all queries=%v", firstQuery(queries), queries)
	}
}
