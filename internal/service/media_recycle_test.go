package service

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestListRecycleBinPrunesOldRowsOverLimit(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	now := time.Now()
	for i := 0; i < maxRecycleBinRecords+5; i++ {
		deletedAt := now.Add(time.Duration(i) * time.Second)
		media := model.Media{
			Base: model.Base{
				ID:        fmt.Sprintf("media-%03d", i),
				DeletedAt: gorm.DeletedAt{Time: deletedAt, Valid: true},
			},
			Title: fmt.Sprintf("Movie %03d", i),
			Path:  filepath.Join(t.TempDir(), fmt.Sprintf("Movie %03d.mkv", i)),
		}
		if err := db.Unscoped().Create(&media).Error; err != nil {
			t.Fatal(err)
		}
	}

	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	rows, err := svc.ListRecycleBin(t.Context(), 500)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != maxRecycleBinRecords {
		t.Fatalf("recycle rows = %d, want %d", len(rows), maxRecycleBinRecords)
	}
	var count int64
	if err := db.Unscoped().Model(&model.Media{}).Where("deleted_at IS NOT NULL").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != maxRecycleBinRecords {
		t.Fatalf("stored recycle rows = %d, want %d", count, maxRecycleBinRecords)
	}
	var oldCount int64
	if err := db.Unscoped().Model(&model.Media{}).Where("id IN ?", []string{"media-000", "media-001", "media-002", "media-003", "media-004"}).Count(&oldCount).Error; err != nil {
		t.Fatal(err)
	}
	if oldCount != 0 {
		t.Fatalf("oldest recycle rows were not pruned, count=%d", oldCount)
	}
}

func TestSoftDeleteInvalidatesMediaAndStatsCache(t *testing.T) {
	db := newServiceTestDB(t, &model.Media{})
	repos := repository.New(db)
	media := model.Media{
		Base:  model.Base{ID: "local-media"},
		Title: "Cached Movie",
		Path:  filepath.Join(t.TempDir(), "Cached Movie.mkv"),
	}
	if err := repos.DB.Create(&media).Error; err != nil {
		t.Fatal(err)
	}

	cache := NewRuntimeCacheService(&config.Config{}, zap.NewNop())
	cache.SetJSON(t.Context(), "media:list:test", map[string]string{"state": "stale"}, time.Minute)
	cache.SetJSON(t.Context(), "stats:snapshot:base", map[string]int{"media": 1}, time.Minute)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos).SetRuntimeCache(cache)
	if err := svc.SoftDelete(t.Context(), media.ID); err != nil {
		t.Fatal(err)
	}

	var mediaCache map[string]string
	if cache.GetJSON(t.Context(), "media:list:test", &mediaCache) {
		t.Fatal("soft delete should invalidate media cache")
	}
	var statsCache map[string]int
	if cache.GetJSON(t.Context(), "stats:snapshot:base", &statsCache) {
		t.Fatal("soft delete should invalidate stats cache")
	}
}
