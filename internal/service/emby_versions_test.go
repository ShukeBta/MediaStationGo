package service

import (
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyLatestItemsIncludesMergedCloudMovieLibrary(t *testing.T) {
	svc := newTestEmbyService(t)
	local := model.Library{Name: "国产电影", Path: `/media/国产电影`, Type: "movie", Enabled: true}
	cloud := model.Library{Name: "OpenList · 国产电影", Path: BuildCloudLibraryPath("openlist", "/国产电影", "/国产电影"), Type: "movie", Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := svc.repo.Library.Create(t.Context(), lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}
	for _, media := range []model.Media{
		{
			Base:      model.Base{ID: "local-movie", CreatedAt: time.Now().Add(-time.Minute)},
			LibraryID: local.ID,
			Title:     "本地版本",
			Path:      `/media/国产电影/local.mkv`,
		},
		{
			Base:      model.Base{ID: "cloud-movie", CreatedAt: time.Now()},
			LibraryID: cloud.ID,
			Title:     "云盘版本",
			Path:      `cloud://openlist/国产电影/cloud.mkv`,
		},
	} {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	latest, err := svc.LatestItems(t.Context(), "user-1", local.ID, 10)
	if err != nil {
		t.Fatalf("latest items: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("latest items = %#v, want local and merged cloud media", latest)
	}
	if latest[0]["Id"] != "cloud-movie" || latest[1]["Id"] != "local-movie" {
		t.Fatalf("latest order/items = %#v, want cloud then local", latest)
	}
}

func TestEmbyMergedLocalCloudMovieVersionsShareMediaSources(t *testing.T) {
	svc := newTestEmbyService(t)
	local := model.Library{Name: "国产电影", Path: `/media/国产电影`, Type: "movie", Enabled: true}
	cloud := model.Library{Name: "OpenList · 国产电影", Path: BuildCloudLibraryPath("openlist", "/国产电影", "/国产电影"), Type: "movie", Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := svc.repo.Library.Create(t.Context(), lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}
	for _, media := range []model.Media{
		{
			Base:      model.Base{ID: "local-version", CreatedAt: time.Now()},
			LibraryID: local.ID,
			Title:     "流浪地球",
			Year:      2019,
			Path:      `/media/国产电影/流浪地球.2019.1080p.mkv`,
			Container: "mkv",
			Width:     1920,
		},
		{
			Base:      model.Base{ID: "cloud-version", CreatedAt: time.Now().Add(time.Minute)},
			LibraryID: cloud.ID,
			Title:     "流浪地球",
			Year:      2019,
			Path:      `cloud://openlist/国产电影/流浪地球.2019.2160p.mkv`,
			Container: "mkv",
			STRMURL:   "https://example.invalid/cloud",
			Width:     3840,
		},
	} {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	items, err := svc.Items(t.Context(), ItemsParams{ParentID: local.ID, IncludeItemTypes: []string{"Movie"}, Recursive: true, Limit: 10})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	rows := items["Items"].([]map[string]any)
	if len(rows) != 1 {
		t.Fatalf("merged local/cloud versions should show as one item, got %#v", rows)
	}
	if rows[0]["Id"] != "local-version" {
		t.Fatalf("local media should be the representative item, got %#v", rows[0])
	}
	sources := rows[0]["MediaSources"].([]map[string]any)
	if len(sources) != 2 {
		t.Fatalf("merged item should expose two media sources, got %#v", sources)
	}

	playback, err := svc.PlaybackInfo(t.Context(), "local-version", "user-1")
	if err != nil {
		t.Fatalf("playback: %v", err)
	}
	playSources := playback["MediaSources"].([]map[string]any)
	if len(playSources) != 2 {
		t.Fatalf("playback should expose local and cloud versions, got %#v", playSources)
	}
}

func TestEmbyLatestItemsCollapsesMovieVersions(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now()
	for _, media := range []model.Media{
		{
			Base:      model.Base{ID: "dune-1080", CreatedAt: now.Add(time.Minute)},
			LibraryID: lib.ID,
			Title:     "Dune",
			Year:      2021,
			TMDbID:    438631,
			Path:      `/media/movies/Dune.2021.1080p.mkv`,
			Width:     1920,
			SizeBytes: 100,
		},
		{
			Base:      model.Base{ID: "dune-2160", CreatedAt: now.Add(2 * time.Minute)},
			LibraryID: lib.ID,
			Title:     "Dune",
			Year:      2021,
			TMDbID:    438631,
			Path:      `/media/movies/Dune.2021.2160p.mkv`,
			Width:     3840,
			SizeBytes: 200,
		},
		{
			Base:      model.Base{ID: "matrix", CreatedAt: now},
			LibraryID: lib.ID,
			Title:     "The Matrix",
			Year:      1999,
			TMDbID:    603,
			Path:      `/media/movies/The.Matrix.1999.mkv`,
		},
	} {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	latest, err := svc.LatestItems(t.Context(), "user-1", lib.ID, 10)
	if err != nil {
		t.Fatalf("latest items: %v", err)
	}
	if len(latest) != 2 {
		t.Fatalf("latest items = %#v, want Dune collapsed plus Matrix", latest)
	}
	if latest[0]["Id"] != "dune-2160" {
		t.Fatalf("latest first item = %#v, want best Dune version", latest[0])
	}
	sources := latest[0]["MediaSources"].([]map[string]any)
	if len(sources) != 2 {
		t.Fatalf("collapsed latest item should expose both versions, got %#v", sources)
	}
}
