package service

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestRecentHistoryPageAppliesVisibilityBeforePagination(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.PlaybackHistory{})
	repos := repository.New(db)
	libraries := []model.Library{
		{Base: model.Base{ID: "library-a"}, Name: "A", Path: "/a", Type: "movie", Enabled: true},
		{Base: model.Base{ID: "library-b"}, Name: "B", Path: "/b", Type: "movie", Enabled: true},
	}
	if err := db.Create(&libraries).Error; err != nil {
		t.Fatal(err)
	}
	media := []model.Media{
		{Base: model.Base{ID: "media-a1"}, LibraryID: "library-a", Title: "A1", Path: "/a/1.mp4"},
		{Base: model.Base{ID: "media-a2"}, LibraryID: "library-a", Title: "A2", Path: "/a/2.mp4"},
		{Base: model.Base{ID: "media-b1"}, LibraryID: "library-b", Title: "B1", Path: "/b/1.mp4"},
	}
	if err := db.Create(&media).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	history := []model.PlaybackHistory{
		{Base: model.Base{ID: "history-a1"}, UserID: "user-1", MediaID: "media-a1", WatchedAt: now.Add(-time.Minute)},
		{Base: model.Base{ID: "history-b1"}, UserID: "user-1", MediaID: "media-b1", WatchedAt: now},
		{Base: model.Base{ID: "history-a2"}, UserID: "user-1", MediaID: "media-a2", WatchedAt: now.Add(-2 * time.Minute)},
	}
	if err := db.Create(&history).Error; err != nil {
		t.Fatal(err)
	}

	playback := NewPlaybackService(zap.NewNop(), repos)
	items, total, err := playback.RecentHistoryPage(t.Context(), "user-1", 1, 1, MediaVisibility{
		IncludeNSFW: true, AllowedLibraryIDs: []string{"library-a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(items) != 1 || items[0].Media == nil || items[0].Media.ID != "media-a1" {
		t.Fatalf("page = %#v total=%d, want latest visible item and total 2", items, total)
	}
}
