package service

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/database"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestSubscriptionCreateRejectsConcurrentDuplicateRules(t *testing.T) {
	cfg := &config.Config{}
	cfg.Database.Type = "sqlite"
	cfg.Database.DBPath = filepath.Join(t.TempDir(), "subscriptions.db")
	cfg.Database.WALMode = true
	cfg.Database.BusyTimeout = 5000
	cfg.Database.MaxOpenConns = 4
	db, err := database.Open(cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, dbErr := db.DB(); dbErr == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	svc := NewSubscriptionService(cfg, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- svc.Create(t.Context(), &model.Subscription{
				UserID:     "user-1",
				Name:       "Example Show",
				FeedURL:    "site-search://search?keyword=Example+Show",
				Filter:     "Example Show",
				MediaType:  "tv",
				Resolution: "1080p",
				Enabled:    true,
			})
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var created, conflicts int
	for createErr := range results {
		switch {
		case createErr == nil:
			created++
		case errors.Is(createErr, ErrSubscriptionAlreadyExists):
			conflicts++
		default:
			t.Fatalf("unexpected create error: %v", createErr)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("created=%d conflicts=%d, want 1/1", created, conflicts)
	}
	var count int64
	if err := db.Model(&model.Subscription{}).Where("archived_at IS NULL").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("active subscription count = %d, want 1", count)
	}
}

func TestSubscriptionUpdateAndRestoreReturnExistingConflict(t *testing.T) {
	db := newServiceTestDB(t, &model.Subscription{})
	if err := database.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	repos := repository.New(db)
	svc := NewSubscriptionService(&config.Config{}, zap.NewNop(), repos, nil, nil, NewHub(zap.NewNop()))
	first := &model.Subscription{UserID: "user-1", Name: "Example", FeedURL: "rss://example", Filter: "Example", Resolution: "1080p", Enabled: true}
	second := &model.Subscription{UserID: "user-1", Name: "Example", FeedURL: "rss://example", Filter: "Example", Resolution: "2160p", Enabled: true}
	if err := svc.Create(t.Context(), first); err != nil {
		t.Fatal(err)
	}
	if err := svc.Create(t.Context(), second); err != nil {
		t.Fatal(err)
	}
	if err := svc.Update(t.Context(), second.ID, map[string]any{"resolution": "1080p"}); !errors.Is(err, ErrSubscriptionAlreadyExists) || SubscriptionAlreadyExistsID(err) != first.ID {
		t.Fatalf("update error = %v, want conflict with %s", err, first.ID)
	}

	archivedAt := time.Now()
	archived := *first
	archived.ID = ""
	archived.ArchivedAt = &archivedAt
	archived.Enabled = false
	if err := repos.Subscription.Create(t.Context(), &archived); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Restore(t.Context(), archived.ID); !errors.Is(err, ErrSubscriptionAlreadyExists) || SubscriptionAlreadyExistsID(err) != first.ID {
		t.Fatalf("restore error = %v, want conflict with %s", err, first.ID)
	}
}
