package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func TestPostgresSeriesBindingConcurrentEpisodeUpdates(t *testing.T) {
	db, cfg := postgresBackupTestDatabase(t)
	for i := 0; i < 2; i++ {
		if err := database.AutoMigrate(db); err != nil {
			t.Fatal(err)
		}
	}
	repos := repository.New(db)
	svc := NewMediaService(cfg, zap.NewNop(), repos)
	NewEmbyService(cfg, zap.NewNop(), repos)
	lib := model.Library{Name: "Series", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Media, 4)
	for i := range rows {
		rows[i] = model.Media{LibraryID: lib.ID, Title: "Shared show", Path: fmt.Sprintf("/media/tv/Shared show/Season 1/S01E%02d.mkv", i+1), SeasonNum: 1, EpisodeNum: i + 1}
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	start := make(chan struct{})
	errs := make(chan error, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			<-start
			for round := 0; round < 3; round++ {
				if err := repos.Media.UpdateWithCurrentSeriesKey(ctx, nil, rows[i].ID, map[string]any{"tm_db_id": 7123, "scrape_status": "matched", "title": fmt.Sprintf("Episode %d revision %d", i, round), "poster_url": fmt.Sprintf("https://example.test/%d-%d.jpg", i, round), "overview": fmt.Sprintf("Synopsis %d", i)}); err != nil {
					errs <- err
					return
				}
			}
		}(i)
	}
	close(start)
	workers.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if _, err := repos.Media.UpdateManyWithCurrentSeriesKeys(ctx, nil, []string{rows[0].ID, rows[1].ID}, map[string]any{"original_name": "Shared show", "tm_db_id": 7124}); err != nil {
		t.Fatal(err)
	}
	cards, total, err := svc.ListLibrarySeriesCards(ctx, lib.ID, 1, 20, MediaVisibility{})
	if err != nil || total != 1 || len(cards) != 1 || cards[0].Count != 4 {
		t.Fatalf("concurrent scrape split series: total=%d cards=%d err=%v", total, len(cards), err)
	}
	for _, id := range []string{rows[2].ID, rows[3].ID} {
		var row model.Media
		if err := db.First(&row, "id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if row.SeriesBindingKey != "tmdb:7124" || row.TMDbID != 0 || row.ScrapeStatus != "pending" {
			t.Fatalf("binding fabricated metadata: %+v", row)
		}
	}
	if err := db.Model(&model.Media{}).Where("id = ?", rows[0].ID).Updates(map[string]any{"path": "/media/tv/Other/S01E01.mkv", "tm_db_id": 9999}).Error; err != nil {
		t.Fatal(err)
	}
	_, total, err = svc.ListLibrarySeriesCards(ctx, lib.ID, 1, 20, MediaVisibility{})
	if err != nil || total != 2 {
		t.Fatalf("direct SQL move kept stale binding: total=%d err=%v", total, err)
	}
}

func TestPostgresSeriesBindingRepairPreservesConcurrentMove(t *testing.T) {
	db, cfg := postgresBackupTestDatabase(t)
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	NewMediaService(cfg, zap.NewNop(), repos)
	lib := model.Library{Name: "Series", Path: "/media/tv", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: lib.ID, Path: "/media/tv/Old work/S01E01.mkv", Title: "Old work", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", TMDbID: 321},
		{LibraryID: lib.ID, Path: "/media/tv/New work/S01E01.mkv", Title: "New work", SeasonNum: 1, EpisodeNum: 1, ScrapeStatus: "matched", TMDbID: 654},
		{LibraryID: lib.ID, Path: "/media/tv/Old work/S01E02.mkv", Title: "Pending episode", SeasonNum: 1, EpisodeNum: 2},
	}
	for i := range rows {
		if err := repos.Media.Upsert(t.Context(), &rows[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&model.Media{}).Where("id = ?", rows[2].ID).UpdateColumn("series_key_version", 0).Error; err != nil {
		t.Fatal(err)
	}
	type repairContextKey struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(t.Context(), repairContextKey{}, true), 20*time.Second)
	defer cancel()
	paused, resume := make(chan struct{}), make(chan struct{})
	var pauseOnce sync.Once
	const callback = "test:pause_series_binding_repair"
	if err := db.Callback().Raw().Before("gorm:raw").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(repairContextKey{}) != true || !strings.Contains(tx.Statement.SQL.String(), "pg_advisory_xact_lock") {
			return
		}
		pauseOnce.Do(func() {
			close(paused)
			select {
			case <-resume:
			case <-ctx.Done():
			}
		})
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Callback().Raw().Remove(callback) })
	finished := make(chan error, 1)
	go func() {
		_, err := repos.Media.BackfillSeriesKeys(ctx, 1)
		finished <- err
	}()
	select {
	case <-paused:
	case <-ctx.Done():
		t.Fatal("repair did not reach its scope lock")
	}
	if err := repos.Media.UpdateWithCurrentSeriesKey(t.Context(), nil, rows[2].ID, map[string]any{"path": "/media/tv/New work/S01E02.mkv"}); err != nil {
		close(resume)
		t.Fatal(err)
	}
	close(resume)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("repair did not finish after the concurrent move")
	}
	var moved model.Media
	if err := db.First(&moved, "id = ?", rows[2].ID).Error; err != nil {
		t.Fatal(err)
	}
	if moved.SeriesBindingKey != "tmdb:654" || boundSeriesIdentity(moved) != "tmdb:654" || moved.TMDbID != 0 || moved.ScrapeStatus != "pending" {
		t.Fatalf("repair replaced the moved episode's current binding: binding=%q identity=%q", moved.SeriesBindingKey, boundSeriesIdentity(moved))
	}
}
