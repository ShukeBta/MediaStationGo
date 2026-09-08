package service

import (
	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTVLookupCachesRequestsNotFileIdentity(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/search/tv" {
			t.Errorf("cross-type request %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"NCIS: Origins","original_name":"NCIS: Origins"},{"id":2,"name":"NCIS","original_name":"NCIS"}]}`))
	}))
	defer server.Close()
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test"
	cfg.Secrets.TMDbAPIProxy = server.URL
	s := &ScraperService{tmdb: NewTMDbProvider(cfg, zap.NewNop(), nil)}
	options := ScrapeOptions{tvLookup: make(map[tvLookupKey]tvLookupResult)}
	for _, path := range []string{"/tv/NCIS/S01/NCIS.S01E01.720p.mkv", "/tv/NCIS/S01/NCIS.S01E01.1080p.mkv"} {
		got, err := s.lookupTVStrict(t.Context(), &model.Media{Path: path}, "NCIS", 0, options)
		if err != nil || got == nil || got.TMDbID != 2 {
			t.Fatalf("got=%+v err=%v", got, err)
		}
		got.Title = "mutated caller copy"
	}
	got, err := s.lookupTVStrict(t.Context(), &model.Media{Path: "/tv/Fringe/Fringe.S01E01.mkv"}, "NCIS", 0, options)
	if got != nil || err != nil || requests != 1 {
		t.Fatalf("cross-file leak: %+v %v requests=%d", got, err, requests)
	}
}

func TestTVLookupAmbiguityAndFailureAreNotNoMatch(t *testing.T) {
	for _, failure := range []bool{false, true} {
		requests := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests++
			if failure {
				w.WriteHeader(503)
				return
			}
			_, _ = w.Write([]byte(`{"results":[{"id":1,"name":"Melrose Place","first_air_date":"1992-07-08"},{"id":2,"name":"Melrose Place","first_air_date":"2009-09-08"}]}`))
		}))
		cfg := &config.Config{}
		cfg.Secrets.TMDbAPIKey = "test"
		cfg.Secrets.TMDbAPIProxy = server.URL
		s := &ScraperService{tmdb: NewTMDbProvider(cfg, zap.NewNop(), nil)}
		options := ScrapeOptions{tvLookup: make(map[tvLookupKey]tvLookupResult)}
		for i := 0; i < 2; i++ {
			if _, err := s.lookupTVStrict(t.Context(), &model.Media{Path: "/tv/Melrose.Place.S01E01.mkv"}, "Melrose Place", 0, options); err == nil {
				t.Fatal("ambiguous/failed request accepted")
			}
		}
		if requests != 1 {
			t.Fatalf("requests=%d", requests)
		}
		server.Close()
	}
}

func TestScrapeDoesNotOverwriteConcurrentEdit(t *testing.T) {
	s, repos, closeServer := newTestScraper(t)
	defer closeServer()
	lib := model.Library{Name: "Movies", Path: "cloud://openlist/movies", Type: "movie"}
	if err := repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	m := model.Media{LibraryID: lib.ID, Path: "cloud://openlist/movies/example.mkv", Title: "Original"}
	if err := repos.DB.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.DB.Model(&model.Media{}).Where("id = ?", m.ID).Update("title", "User edit").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.applyProviderMatchWithOptions(t.Context(), &m, &lib, &Match{Title: "Scraped", MediaType: "movie"}, ScrapeOptions{}); err == nil {
		t.Fatal("stale write succeeded")
	}
	stored, err := repos.Media.FindByID(t.Context(), m.ID)
	if err != nil || stored.Title != "User edit" {
		t.Fatalf("concurrent edit lost: %+v %v", stored, err)
	}
}
