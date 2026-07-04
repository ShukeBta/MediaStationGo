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

func TestOrganizeDirectoryReclassifiesExistingWrongCategoryMedia(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id":                292696,
				"name":              "莫离",
				"original_name":     "The First Jasmine",
				"original_language": "zh",
				"origin_country":    []string{"CN"},
				"genre_ids":         []int{18},
				"first_air_date":    "2026-06-23",
			}},
		})
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	srcRoot := filepath.Join(root, "downloads")
	dest := filepath.Join(root, "media")
	sourceFile := filepath.Join(srcRoot, "欧美剧", "The.First.Jasmine.S01.1080p.TX.WEB-DL.AAC2.0.H.264-MWeb", "The.First.Jasmine.S01E01.1080p.TX.WEB-DL.AAC2.0.H.264-MWeb.mkv")
	writeOrgFile(t, sourceFile, "episode")

	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	domesticLib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &euusLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &domesticLib); err != nil {
		t.Fatal(err)
	}

	wrongPath := filepath.Join(euusLib.Path, "The First Jasmine", "Season 01", "The First Jasmine - S01E01.mkv")
	writeOrgFile(t, wrongPath, "existing")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    euusLib.ID,
		Title:        "莫离",
		OriginalName: "The First Jasmine",
		Path:         wrongPath,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       292696,
		Languages:    "zh",
		Countries:    "CN",
		Genres:       "剧情",
		ScrapeStatus: "matched",
	}).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.OrganizeDirectory(t.Context(), OrganizeOptions{
		SourcePath:   srcRoot,
		DestPath:     euusLib.Path,
		TransferMode: TransferCopy,
	})
	if err != nil {
		t.Fatalf("organize directory: %v", err)
	}
	want := filepath.Join(domesticLib.Path, "莫离", "Season 01", "莫离 - S01E01.mkv")
	if res.Reclassified != 1 || res.Organized != 0 || res.Skipped != 0 {
		t.Fatalf("result = %+v, want reclassified=1 only", res)
	}
	if _, err := os.Stat(wrongPath); !os.IsNotExist(err) {
		t.Fatalf("wrong category path should be moved away, stat err=%v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("reclassified media missing at %q: %v; items=%#v", want, err, res.Items)
	}
	if _, err := os.Stat(sourceFile); err != nil {
		t.Fatalf("source download should remain untouched: %v", err)
	}

	var got model.Media
	if err := repos.DB.First(&got, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != domesticLib.ID {
		t.Fatalf("library_id = %q, want domestic library %q", got.LibraryID, domesticLib.ID)
	}
}

func TestReclassifyMisclassifiedMediaFiltersByMediaID(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	domesticLib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &euusLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &domesticLib); err != nil {
		t.Fatal(err)
	}

	movingPath := filepath.Join(euusLib.Path, "Motherhood Of Taihang", "Season 01", "Motherhood Of Taihang - S01E01.mkv")
	stayingPath := filepath.Join(euusLib.Path, "The First Jasmine", "Season 01", "The First Jasmine - S01E01.mkv")
	writeOrgFile(t, movingPath, "episode")
	writeOrgFile(t, stayingPath, "episode")

	moving := model.Media{
		LibraryID:    euusLib.ID,
		Title:        "太行谣",
		OriginalName: "Motherhood Of Taihang",
		Path:         movingPath,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       323682,
		Languages:    "zh",
		Countries:    "CN",
		Genres:       "剧情",
		ScrapeStatus: "matched",
	}
	staying := model.Media{
		LibraryID:    euusLib.ID,
		Title:        "莫离",
		OriginalName: "The First Jasmine",
		Path:         stayingPath,
		SeasonNum:    1,
		EpisodeNum:   1,
		TMDbID:       292696,
		Languages:    "zh",
		Countries:    "CN",
		Genres:       "剧情",
		ScrapeStatus: "matched",
	}
	if err := repos.DB.Create(&moving).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Create(&staying).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	res, err := organizer.ReclassifyMisclassifiedMedia(t.Context(), MediaCategoryReclassifyOptions{MediaIDs: []string{moving.ID}})
	if err != nil {
		t.Fatalf("reclassify media: %v", err)
	}
	want := filepath.Join(domesticLib.Path, "太行谣", "Season 01", "太行谣 - S01E01.mkv")
	if res.Reclassified != 1 {
		t.Fatalf("reclassified = %d, want 1; items=%#v errors=%#v", res.Reclassified, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("selected media should move to domestic path %q: %v", want, err)
	}
	if _, err := os.Stat(stayingPath); err != nil {
		t.Fatalf("unselected media should stay at wrong path for single-media reclassify: %v", err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "id = ?", staying.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != euusLib.ID || got.Path != stayingPath {
		t.Fatalf("unselected row changed: %#v", got)
	}
}

func TestReclassifyMisclassifiedMediaRetriesNoMatchMetadata(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/search/tv" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{
				"id":                292696,
				"name":              "莫离",
				"original_name":     "The First Jasmine",
				"original_language": "zh",
				"origin_country":    []string{"CN"},
				"genre_ids":         []int{18},
				"first_air_date":    "2026-06-23",
			}},
		})
	}))
	defer upstream.Close()

	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = upstream.URL
	scraper := NewScraperService(cfg, zap.NewNop(), repos, NewTMDbProvider(cfg, zap.NewNop(), nil), nil, nil, nil, NewHub(zap.NewNop()))

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	domesticLib := model.Library{Name: "国产剧", Path: filepath.Join(dest, "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &euusLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &domesticLib); err != nil {
		t.Fatal(err)
	}

	wrongPath := filepath.Join(euusLib.Path, "The First Jasmine", "Season 1", "The First Jasmine - S01E01.mkv")
	writeOrgFile(t, wrongPath, "episode")
	if err := repos.DB.Create(&model.Media{
		LibraryID:    euusLib.ID,
		Title:        "the first jasmine",
		Path:         wrongPath,
		SeasonNum:    1,
		EpisodeNum:   1,
		ScrapeStatus: "no_match",
	}).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	organizer.SetScraper(scraper)
	res, err := organizer.ReclassifyMisclassifiedMedia(t.Context(), MediaCategoryReclassifyOptions{})
	if err != nil {
		t.Fatalf("reclassify media: %v", err)
	}
	want := filepath.Join(domesticLib.Path, "莫离", "Season 01", "莫离 - S01E01.mkv")
	if res.Reclassified != 1 {
		t.Fatalf("reclassified = %d, want 1; items=%#v errors=%#v", res.Reclassified, res.Items, res.Errors)
	}
	if _, err := os.Stat(wrongPath); !os.IsNotExist(err) {
		t.Fatalf("wrong no_match path should move away, stat err=%v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("reclassified media missing at %q: %v; items=%#v", want, err, res.Items)
	}
	var got model.Media
	if err := repos.DB.First(&got, "path = ?", want).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != domesticLib.ID || got.Title != "莫离" || got.TMDbID != 292696 || got.Countries != "CN" || got.Languages != "zh" || got.ScrapeStatus != "matched" {
		t.Fatalf("row after metadata retry = %#v, want domestic matched metadata", got)
	}
}

func TestReclassifyMisclassifiedMediaHonorsManualMovieHint(t *testing.T) {
	repos := newOrganizerTestRepo(t)
	cfg := &config.Config{}
	cfg.Organizer.SmartClassify = true

	root := t.TempDir()
	dest := filepath.Join(root, "media")
	euusLib := model.Library{Name: "欧美剧", Path: filepath.Join(dest, "电视剧", "欧美剧"), Type: "tv", Enabled: true}
	foreignMovieLib := model.Library{Name: "欧美电影", Path: filepath.Join(dest, "电影", "欧美电影"), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &euusLib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &foreignMovieLib); err != nil {
		t.Fatal(err)
	}

	wrongPath := filepath.Join(euusLib.Path, "Dune", "Season 01", "Dune - S01E202.mkv")
	writeOrgFile(t, wrongPath, "movie")
	media := model.Media{
		LibraryID:    euusLib.ID,
		Title:        "Dune",
		OriginalName: "Dune",
		Path:         wrongPath,
		SeasonNum:    1,
		EpisodeNum:   202,
		TMDbID:       438631,
		Languages:    "en",
		Countries:    "US",
		Genres:       "科幻,冒险",
		Year:         2021,
		ScrapeStatus: "matched",
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	organizer := NewOrganizerService(cfg, zap.NewNop(), repos)
	res, err := organizer.ReclassifyMisclassifiedMedia(t.Context(), MediaCategoryReclassifyOptions{
		MediaIDs:       []string{media.ID},
		MediaTypeHints: map[string]string{media.ID: "movie"},
	})
	if err != nil {
		t.Fatalf("reclassify media: %v", err)
	}
	want := filepath.Join(foreignMovieLib.Path, "Dune (2021)", "Dune (2021).mkv")
	if res.Reclassified != 1 {
		t.Fatalf("reclassified = %d, want 1; items=%#v errors=%#v", res.Reclassified, res.Items, res.Errors)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("manual movie reclassify target missing at %q: %v", want, err)
	}
	var got model.Media
	if err := repos.DB.First(&got, "id = ?", media.ID).Error; err != nil {
		t.Fatal(err)
	}
	if got.LibraryID != foreignMovieLib.ID || got.Path != want || got.SeasonNum != 0 || got.EpisodeNum != 0 || got.SeriesID != "" {
		t.Fatalf("row after manual movie hint = %#v, want foreign movie library with episode markers cleared", got)
	}
}
