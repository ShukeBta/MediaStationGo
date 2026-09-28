package service

import (
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

// Uses a disposable database, never the database named in the connection URL.
func TestPostgresAceFeatureIntegration(t *testing.T) {
	db, cfg := postgresBackupTestDatabase(t)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
	repos := repository.New(db)
	mediaService := NewMediaService(cfg, zap.NewNop(), repos)
	if err := NewAPIConfigService(zap.NewNop(), repos, nil).SeedDefaults(t.Context()); err != nil {
		t.Fatalf("seed provider schema: %v", err)
	}
	lib := model.Library{Name: "Port integration", Path: t.TempDir(), Type: "movie"}
	if err := db.Create(&lib).Error; err != nil {
		t.Fatal(err)
	}
	m := model.Media{LibraryID: lib.ID, Path: filepath.Join(lib.Path, "movie.mkv"), Title: "Manual title", Overview: "Manual description", DoubanID: "123", DurationSec: 1200}
	if err := db.Create(&m).Error; err != nil {
		t.Fatal(err)
	}
	t.Run("douban enrichment and person credits", func(t *testing.T) {
		provider := NewDoubanProvider(nil, nil)
		provider.client = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return doubanFixtureResponse(req, 200, doubanFullFixture), nil
		})}
		scraper := &ScraperService{repo: repos, douban: provider, log: zap.NewNop()}
		got, err := scraper.EnrichFromDouban(t.Context(), m.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Title != m.Title || got.Overview != m.Overview || got.DoubanRating != 8.6 {
			t.Fatalf("enrichment: %+v", got)
		}
		match := &Match{People: []PersonMetadata{{Name: "Alex", Source: "tmdb", SourceID: "101", Type: "Actor", Role: "Doctor"}, {Name: "Alex", Source: "tmdb", SourceID: "102", Type: "Director"}}}
		if err := scraper.persistMediaPeople(t.Context(), m.ID, match); err != nil {
			t.Fatal(err)
		}
		if err := scraper.persistMediaPeople(t.Context(), m.ID, match); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := db.Model(&model.Person{}).Where("name = ?", "Alex").Count(&count).Error; err != nil || count != 2 {
			t.Fatalf("people identities: %d %v", count, err)
		}
	})
	t.Run("TMDb snapshot conflicts and lease", func(t *testing.T) {
		catalog := repository.NewTMDbCatalogRepository(db)
		item := model.TMDbCatalogItem{Key: "movie/42", Kind: "movie", TMDbID: 42, Complete: true, Expanded: true, Snapshot: `{"id":42,"credits":{"cast":[]}}`, FetchedAt: time.Now().UTC()}
		if err := catalog.Save(t.Context(), []model.TMDbCatalogItem{item}); err != nil {
			t.Fatal(err)
		}
		basic := item
		basic.Expanded = false
		basic.Snapshot = `{"id":42}`
		if err := catalog.Save(t.Context(), []model.TMDbCatalogItem{basic}); err != nil {
			t.Fatal(err)
		}
		stored, err := catalog.Find(t.Context(), item.Key)
		if err != nil || stored.Snapshot != item.Snapshot {
			t.Fatalf("rich snapshot lost: %+v %v", stored, err)
		}
		job, err := catalog.Claim(t.Context(), item.Key, time.Now().UTC(), false)
		if err != nil || job == nil {
			t.Fatalf("lease: %+v %v", job, err)
		}
		if second, err := catalog.Claim(t.Context(), item.Key, time.Now().UTC(), true); err != nil || second != nil {
			t.Fatalf("duplicate lease: %+v %v", second, err)
		}
		if err := catalog.Finish(t.Context(), job, "ready", "", time.Now().Add(time.Hour), true); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("NFO writeback and playback aggregates", func(t *testing.T) {
		title := "Edited title"
		if _, err := mediaService.UpdateMetadata(t.Context(), m.ID, MediaMetadataUpdate{Title: &title, WriteNFO: true}); err != nil {
			t.Fatal(err)
		}
		doc, _, err := readNFO(nfoPath(m.Path))
		if err != nil || doc.Title != title {
			t.Fatalf("NFO: %+v %v", doc, err)
		}
		container := &Container{Repo: repos}
		for range 2 {
			if err := container.RecordPlaybackEvent(t.Context(), "viewer", m.ID, "one-session", "Web", 60000, 1200000); err != nil {
				t.Fatal(err)
			}
		}
		from := time.Now().UTC().Truncate(24 * time.Hour)
		for _, grain := range []string{"day", "week", "month"} {
			stats, err := repos.PlaybackStats(t.Context(), repository.PlaybackStatsFilter{From: from, To: from.Add(24 * time.Hour), RankFrom: from, RankTo: from.Add(24 * time.Hour), Grain: grain, Page: 1, PageSize: 10})
			if err != nil || stats.Total != 1 || len(stats.Buckets) != 1 || len(stats.Ranking) != 1 {
				t.Fatalf("%s stats: %+v %v", grain, stats, err)
			}
		}
	})
	t.Run("probe storage and source cleanup trigger", func(t *testing.T) {
		current, err := repos.Media.FindByID(t.Context(), m.ID)
		if err != nil {
			t.Fatal(err)
		}
		doc := &ProbeDocument{SchemaVersion: 1, Streams: []ProbeStream{{Index: 0, CodecType: "video", CodecName: "hevc"}, {Index: 3, CodecType: "audio", CodecName: "aac", Tags: ProbeTags{Language: "ja"}}}}
		if err := saveMediaProbeDocument(t.Context(), repos, current, doc); err != nil {
			t.Fatal(err)
		}
		if loaded := loadMediaProbeDocument(t.Context(), repos, current); loaded == nil || len(loaded.Streams) != 2 || loaded.Streams[1].Index != 3 {
			t.Fatalf("probe: %+v", loaded)
		}
		if err := db.Model(&model.Media{}).Where("id = ?", m.ID).Update("strm_url", "https://example.test/new-source").Error; err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := db.Model(&model.MediaProbeMetadata{}).Where("media_id = ?", m.ID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("stale probe rows: %d %v", count, err)
		}
	})
	t.Run("durable task history and UTC daily logs", func(t *testing.T) {
		tracker := NewTaskTrackerService(zap.NewNop(), nil)
		if err := tracker.SetPersistence(db); err != nil {
			t.Fatal(err)
		}
		handle := tracker.Start("integration", "Postgres integration", TaskUpdate{Message: "start"})
		handle.Update(TaskUpdate{Details: []string{"scanned file", "token=must-not-persist"}})
		handle.Finish(nil, TaskUpdate{Message: "done"})
		from := time.Now().UTC().Truncate(24 * time.Hour)
		filter := repository.TaskHistoryFilter{From: from, To: from.Add(24 * time.Hour), Page: 1, PageSize: 1, Kind: "integration"}
		days, err := repos.TaskLogDays(t.Context(), filter)
		if err != nil || len(days) != 1 {
			t.Fatalf("log days: %+v %v", days, err)
		}
		logs, total, err := repos.TaskLogs(t.Context(), filter)
		if err != nil || len(logs) != 1 || total < 2 {
			t.Fatalf("logs: %+v %d %v", logs, total, err)
		}
		restarted := NewTaskTrackerService(zap.NewNop(), nil)
		if err := restarted.SetPersistence(db); err != nil {
			t.Fatal(err)
		}
		if recent := restarted.Snapshot().Recent; len(recent) != 1 || recent[0].Status != TaskStatusCompleted {
			t.Fatalf("recovered: %+v", recent)
		}
	})
	t.Run("automatic multipart groups fit persistent keys", func(t *testing.T) {
		parts := []model.Media{
			{LibraryID: lib.ID, Title: "Movie", Path: filepath.Join(lib.Path, "Movie Part 1.mkv")},
			{LibraryID: lib.ID, Title: "Movie", Path: filepath.Join(lib.Path, "Movie Part 2.mkv")},
		}
		if err := db.Create(&parts).Error; err != nil {
			t.Fatal(err)
		}
		scanner := &ScannerService{repo: repos}
		if _, err := scanner.reconcileMediaParts(t.Context(), lib.ID, lib.Path); err != nil {
			t.Fatal(err)
		}
		first, err := repos.Media.FindByID(t.Context(), parts[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		second, err := repos.Media.FindByID(t.Context(), parts[1].ID)
		if err != nil || first.PartGroupKey == "" || len(first.PartGroupKey) > 64 || first.PartGroupKey != second.PartGroupKey || first.PartIndex != 1 || second.PartIndex != 2 {
			t.Fatalf("parts: %+v %+v %v", first, second, err)
		}
	})
}
