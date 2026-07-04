package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestOrganizeDirectoryRejectsWrongYearScraperRename(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/search/tv":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{{
					"id":             999,
					"name":           "Parade of Stars Auto Show",
					"first_air_date": "1952-01-01",
				}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Auto.Show.S01E03.2026.1080p.mkv")
	writeOrgFile(t, sourceFile, "episode")
	secondSourceFile := filepath.Join(src, "Auto.Show.S01E04.2026.1080p.mkv")
	writeOrgFile(t, secondSourceFile, "episode")

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
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

	rejected := filepath.Join(dest, "电视剧", "Parade of Stars Auto Show", "Season 01", "Parade of Stars Auto Show - S01E03.mkv")
	if _, err := os.Stat(rejected); err == nil {
		t.Fatalf("wrong-year metadata match should not rename to %q", rejected)
	}
	want := filepath.Join(dest, "电视剧", "Auto Show", "Season 01", "Auto Show - S01E03.mkv")
	if res.Organized != 2 {
		t.Fatalf("organized = %d, want 2; items=%#v errors=%#v", res.Organized, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("organize should keep parsed title at %q: %v; items=%#v", want, err, res.Items)
	}
	secondWant := filepath.Join(dest, "电视剧", "Auto Show", "Season 01", "Auto Show - S01E04.mkv")
	if _, err := os.Stat(secondWant); err != nil {
		t.Fatalf("organize should not reuse rejected cached match at %q: %v; items=%#v", secondWant, err, res.Items)
	}
}

func TestOrganizeDirectoryDoesNotUseEpisodeNFOTitleAsSeriesTitle(t *testing.T) {
	var queries []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Query().Get("query"))
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("query") != "哈哈哈哈哈" {
			_ = json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id":             112732,
				"name":           "哈哈哈哈哈",
				"first_air_date": "2026-01-01",
			}},
		})
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "哈哈哈哈哈", "Season 06", "哈哈哈哈哈 - S06E11.mkv")
	writeOrgFile(t, sourceFile, "episode")
	if err := os.WriteFile(nfoPath(sourceFile), []byte(`<episodedetails><title>第 11 集</title><season>6</season><episode>11</episode><tmdbid>7129825</tmdbid></episodedetails>`), 0o644); err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
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
		t.Fatalf("organized = %d, want 1; items=%#v errors=%#v queries=%v", res.Organized, res.Items, res.Errors, queries)
	}
	rejected := filepath.Join(dest, "电视剧", "第 11 集", "Season 06", "第 11 集 - S06E11.mkv")
	if _, err := os.Stat(rejected); err == nil {
		t.Fatalf("episode title should not be used as series folder: %q", rejected)
	}
	want := filepath.Join(dest, "电视剧", "哈哈哈哈哈", "Season 06", "哈哈哈哈哈 - S06E11.mkv")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("expected organized file at %q: %v; items=%#v queries=%v", want, err, res.Items, queries)
	}
	for _, query := range queries {
		if unsafeAutomaticEpisodeQuery(query) {
			t.Fatalf("organizer queried unsafe episode title %q; all queries=%v", query, queries)
		}
	}
}

func TestOrganizeDirectoryDedupsByExternalIDBeforeRename(t *testing.T) {
	scraper, repos, closeServer := newTestScraper(t)
	defer closeServer()

	root := t.TempDir()
	src := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(src, "Spy.x.Family.S01E01.2022.2160p.mkv")
	writeOrgFile(t, sourceFile, "episode")

	existingPath := filepath.Join(dest, "电视剧", "旧错误名", "Season 01", "旧错误名 - S01E01.mkv")
	writeOrgFile(t, existingPath, "existing")
	lib := model.Library{Name: "剧集", Path: filepath.Join(dest, "电视剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&model.Media{
		LibraryID:    lib.ID,
		Title:        "旧错误名",
		Path:         existingPath,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       12345,
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}

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
	if res.Organized != 0 || res.Skipped != 1 {
		t.Fatalf("organize result = organized %d skipped %d, want 0/1; items=%#v errors=%#v", res.Organized, res.Skipped, res.Items, res.Errors)
	}
	if len(res.Items) != 1 || res.Items[0].Reason != organizeSkipDuplicateLibrary {
		t.Fatalf("source should be skipped as external-id duplicate: %#v", res.Items)
	}
	if _, err := os.Stat(sourceFile); err != nil {
		t.Fatalf("duplicate source should remain untouched: %v", err)
	}
}
