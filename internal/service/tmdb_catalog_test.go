package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newCatalogTestService(t *testing.T, handler http.Handler) *TMDbCatalogService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.Media{}, &model.Library{}, &model.TMDbCatalogItem{}, &model.TMDbCatalogJob{}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	cfg := &config.Config{}
	cfg.Secrets.TMDbAPIKey = "test-key"
	cfg.Secrets.TMDbAPIProxy = server.URL
	tmdb := NewTMDbProvider(cfg, zap.NewNop(), nil)
	return NewTMDbCatalogService(repository.New(db), tmdb, NewTaskTrackerService(zap.NewNop(), nil))
}

func TestTMDbCatalogRecheckPersistsFullCatalogAndProtectsLocalEdits(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	s := newCatalogTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/tv/1":
			fmt.Fprint(w, `{"id":1,"name":"Series","seasons":[{"id":11,"season_number":1,"episode_count":2},{"id":12,"season_number":2,"episode_count":1}]}`)
		case "/tv/1/season/1":
			fmt.Fprint(w, `{"id":11,"name":"Season One","season_number":1,"air_date":"2026-09-01","episodes":[{"id":101,"season_number":1,"episode_number":1,"name":"Pilot","overview":"Provider description","air_date":"2026-09-01","still_path":"/1.jpg"},{"id":102,"season_number":1,"episode_number":2,"name":"Second","air_date":"2026-09-08"}]}`)
		case "/tv/1/season/2":
			fmt.Fprint(w, `{"id":12,"name":"Season Two","season_number":2,"episodes":[{"id":201,"season_number":2,"episode_number":1,"name":"Future"}]}`)
		case "/movie/1":
			fmt.Fprint(w, `{"id":1,"title":"Different movie with same ID"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	ctx := context.Background()
	for _, lib := range []model.Library{{Base: model.Base{ID: "tv"}, Name: "TV", Path: "/tv", Type: "tv"}, {Base: model.Base{ID: "movie"}, Name: "Movies", Path: "/movie", Type: "movie"}} {
		if err := s.repos.DB.Create(&lib).Error; err != nil {
			t.Fatal(err)
		}
	}
	media := model.Media{Base: model.Base{ID: "local"}, LibraryID: "tv", Path: "/tv/s01e01.mkv", Title: "Local title", Overview: "Manual description", TMDbID: 1, SeasonNum: 1, EpisodeNum: 1}
	for _, item := range []model.Media{media, {Base: model.Base{ID: "movie"}, LibraryID: "movie", Path: "/movie/1.mkv", Title: "Movie", TMDbID: 1}, {Base: model.Base{ID: "missing"}, LibraryID: "tv", Path: "/tv/s09e01.mkv", Title: "Unknown season", TMDbID: 1, SeasonNum: 9, EpisodeNum: 1}} {
		if err := s.repos.DB.Create(&item).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.RunCatalogMaintenance(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := s.RunCatalogMaintenance(ctx, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.repos.Media.FindByID(ctx, "local")
	if got.EpisodeTitle != "Pilot" || got.Overview != "Manual description" || got.ReleaseDate != "2026-09-01" {
		t.Fatalf("metadata enrichment incorrect: %#v", got)
	}
	catalog, err := s.SeriesCatalog(ctx, &media, nil)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Series == nil || len(catalog.Seasons) != 3 || len(catalog.Seasons[0].Episodes) != 2 || !catalog.Seasons[0].Episodes[0].Available || catalog.Seasons[0].Episodes[1].Available || catalog.Seasons[1].Episodes[0].Available {
		t.Fatalf("missing placeholders/availability: %#v", catalog)
	}
	var count int64
	s.repos.DB.Model(&model.Media{}).Count(&count)
	if count != 3 {
		t.Fatalf("catalog wrote playable placeholder rows: %d", count)
	}
	job, _ := s.catalog.State(ctx, "tv/1/season/9")
	if job.Status != "not_found" || job.DueAt.Before(time.Now()) {
		t.Fatalf("404 retry lost: %#v", job)
	}
	if snapshot, _ := s.catalog.Find(ctx, "movie/1"); snapshot == nil {
		t.Fatal("movie/TV IDs were conflated")
	}
	if err := s.RunCatalogMaintenance(ctx, false); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for path, count := range calls {
		if count != 1 {
			t.Fatalf("re-requested %s during cooldown: %d", path, count)
		}
	}
}

func TestTMDbCatalogSnapshotBackfillUsesExistingRootAndOnlyFetchesMissingSeason(t *testing.T) {
	var calls []string
	s := newCatalogTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.URL.Path != "/tv/7/season/1" {
			http.Error(w, "unexpected existing snapshot fetch", http.StatusInternalServerError)
			return
		}
		fmt.Fprint(w, `{"id":71,"season_number":1,"episodes":[{"id":711,"season_number":1,"episode_number":1,"name":"Backfilled"}]}`)
	}))
	ctx := context.Background()
	lib := model.Library{Base: model.Base{ID: "tv"}, Name: "TV", Type: "tv", Path: "/tv"}
	if err := s.repos.DB.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	media := model.Media{LibraryID: lib.ID, Path: "/tv/7.mkv", Title: "Local", TMDbID: 7, SeasonNum: 1, EpisodeNum: 1}
	if err := s.repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.tmdb.persistCatalogResponse(ctx, "/tv/7", []byte(`{"id":7,"name":"Saved root","seasons":[{"id":71,"season_number":1}]}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.RunCatalogMaintenance(ctx, true); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || calls[0] != "/tv/7/season/1" {
		t.Fatalf("backfill fetched existing root: %#v", calls)
	}
	if err := s.RunCatalogMaintenance(ctx, true); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 {
		t.Fatalf("repeat backfill was not idempotent: %#v", calls)
	}
	item, _ := s.catalog.Find(ctx, "tv/7/season/1/episode/1")
	if item == nil || item.Title != "Backfilled" || item.Snapshot == "" {
		t.Fatalf("missing episode snapshot: %#v", item)
	}
}

func TestTMDbCatalogRejectsWrongSeasonIdentityAndPreservesSnapshot(t *testing.T) {
	s := newCatalogTestService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":20,"season_number":2,"episodes":[]}`)
	}))
	ctx := context.Background()
	if err := s.catalog.Save(ctx, []model.TMDbCatalogItem{{Key: "tv/1/season/1", TMDbID: 10, Kind: "season", RootID: 1, SeasonNum: 1, Snapshot: `{"id":10,"season_number":1}`, Complete: true}}); err != nil {
		t.Fatal(err)
	}
	err := s.recheckCatalogKey(ctx, "tv/1/season/1", time.Now(), true, false, map[string]int64{})
	if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("wrong identity accepted: %v", err)
	}
	got, _ := s.catalog.Find(ctx, "tv/1/season/1")
	if got.TMDbID != 10 {
		t.Fatal("wrong identity replaced snapshot")
	}
	job, _ := s.catalog.State(ctx, "tv/1/season/1")
	if job.Status != "retry" || job.LeaseToken != "" {
		t.Fatalf("failed lease not released: %#v", job)
	}
}
