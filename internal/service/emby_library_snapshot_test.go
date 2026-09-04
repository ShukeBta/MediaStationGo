package service

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyLoginListsLoadLibrariesOncePerRequest(t *testing.T) {
	svc := newTestEmbyService(t)
	library := model.Library{Name: "剧集", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &library); err != nil {
		t.Fatalf("create library: %v", err)
	}
	createdAt := time.Now().UTC()
	for episode := 1; episode <= 12; episode++ {
		media := model.Media{
			Base:         model.Base{ID: fmt.Sprintf("episode-%02d", episode), CreatedAt: createdAt.Add(time.Duration(episode) * time.Minute)},
			LibraryID:    library.ID,
			SeriesID:     "series-1",
			Title:        "Query Count Series",
			EpisodeTitle: fmt.Sprintf("Episode %d", episode),
			Path:         fmt.Sprintf(`/media/tv/Query Count/Season 01/S01E%02d.mkv`, episode),
			SeasonNum:    1,
			EpisodeNum:   episode,
			TMDbID:       1000 + episode,
		}
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create episode %d: %v", episode, err)
		}
	}

	libraryQueries := 0
	if err := svc.repo.DB.Callback().Query().Before("gorm:query").Register("test:count-emby-login-library-queries", func(tx *gorm.DB) {
		if tx.Statement.Table == "libraries" {
			libraryQueries++
		}
	}); err != nil {
		t.Fatalf("register query counter: %v", err)
	}

	assertOneLibraryQuery := func(name string, call func() error) {
		t.Helper()
		libraryQueries = 0
		if err := call(); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if libraryQueries != 1 {
			t.Fatalf("%s libraries queries = %d, want 1", name, libraryQueries)
		}
	}

	assertOneLibraryQuery("Items", func() error {
		_, err := svc.Items(t.Context(), ItemsParams{
			Recursive:        true,
			IncludeItemTypes: []string{"Movie", "Episode"},
			Limit:            10,
			OmitMediaSources: true,
		})
		return err
	})
	assertOneLibraryQuery("LatestItems", func() error {
		svc.InvalidateUserVisibility("")
		_, err := svc.LatestItems(t.Context(), "", "", 10)
		return err
	})
}
