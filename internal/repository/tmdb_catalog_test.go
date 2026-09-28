package repository

import (
	"context"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func catalogTestRepository(t *testing.T) *TMDbCatalogRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&model.TMDbCatalogItem{}, &model.TMDbCatalogJob{}); err != nil {
		t.Fatal(err)
	}
	return NewTMDbCatalogRepository(db)
}

func TestTMDbCatalogSnapshotPreservesDetailAndRollsBack(t *testing.T) {
	r, ctx := catalogTestRepository(t), context.Background()
	full := model.TMDbCatalogItem{Key: "tv/1/season/1/episode/1", TMDbID: 100, Kind: "episode", Snapshot: `{"id":100,"credits":{"cast":[]}}`, Complete: true, Expanded: true, Title: "Complete"}
	if err := r.Save(ctx, []model.TMDbCatalogItem{full}); err != nil {
		t.Fatal(err)
	}
	outline := full
	outline.Complete = false
	outline.Snapshot = `{"id":100}`
	outline.Title = "Outline"
	if err := r.Save(ctx, []model.TMDbCatalogItem{outline}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Find(ctx, full.Key)
	if err != nil || got.Title != "Complete" || got.Snapshot != full.Snapshot {
		t.Fatalf("detail overwritten: %#v, %v", got, err)
	}
	plain := full
	plain.Expanded = false
	plain.Snapshot = `{"id":100,"name":"Fresh"}`
	if err := r.Save(ctx, []model.TMDbCatalogItem{plain}); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Find(ctx, full.Key)
	if got.Snapshot != full.Snapshot || !got.Expanded {
		t.Fatal("plain detail discarded appended metadata")
	}
	full.Title = "Must roll back"
	if err := r.Save(ctx, []model.TMDbCatalogItem{full, {Key: "invalid", TMDbID: 2, Snapshot: `{`}}); err == nil {
		t.Fatal("expected malformed snapshot to fail")
	}
	got, _ = r.Find(ctx, full.Key)
	if got.Title != "Complete" {
		t.Fatalf("partial transaction persisted: %s", got.Title)
	}
}

func TestTMDbCatalogLeaseAndCooldownSurviveNewRepository(t *testing.T) {
	r, ctx := catalogTestRepository(t), context.Background()
	cutoff := time.Now().UTC()
	first, err := r.Claim(ctx, "tv/1", cutoff, false)
	if err != nil || first == nil {
		t.Fatalf("claim: %v", err)
	}
	restarted := NewTMDbCatalogRepository(r.db)
	second, err := restarted.Claim(ctx, "tv/1", cutoff, true)
	if err != nil || second != nil {
		t.Fatalf("active lease stolen: %v %v", second, err)
	}
	if err := r.Finish(ctx, first, "pending", "", cutoff.Add(24*time.Hour), true); err != nil {
		t.Fatal(err)
	}
	if job, err := restarted.Claim(ctx, "tv/1", cutoff, false); err != nil || job != nil {
		t.Fatalf("cooldown lost: %v %v", job, err)
	}
	manual, err := restarted.Claim(ctx, "tv/1", cutoff, true)
	if err != nil || manual == nil {
		t.Fatalf("manual refresh: %v", err)
	}
	if err := r.Finish(ctx, first, "pending", "old worker", cutoff, true); err == nil {
		t.Fatal("stale worker overwrote newer lease")
	}
	if err := r.Finish(ctx, manual, "retry", "transient error", cutoff.Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	state, _ := restarted.State(ctx, "tv/1")
	if state.Status != "retry" || state.Attempts != 1 || state.LeaseToken != "" || state.CheckedAt == nil {
		t.Fatalf("wrong persisted state: %#v", state)
	}
}
