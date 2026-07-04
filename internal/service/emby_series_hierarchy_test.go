package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyItemsExposeSeriesSeasonEpisodeHierarchy(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "番剧", Path: `F:\downloads\日番`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	for _, media := range []model.Media{
		{
			Base:         model.Base{ID: "ep-1"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家",
			OriginalName: "SPY×FAMILY",
			EpisodeTitle: "第 1 集",
			Path:         `F:\downloads\日番\剧集\间谍过家家\Season 02\间谍过家家 - S02E01.mkv`,
			PosterURL:    `F:\poster.jpg`,
			SeasonNum:    2,
			EpisodeNum:   1,
		},
		{
			Base:         model.Base{ID: "ep-2"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家",
			OriginalName: "SPY×FAMILY",
			EpisodeTitle: "第 2 集",
			Path:         `F:\downloads\日番\剧集\间谍过家家\Season 02\间谍过家家 - S02E02.mkv`,
			PosterURL:    `F:\poster.jpg`,
			SeasonNum:    2,
			EpisodeNum:   2,
		},
	} {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	rootItems := root["Items"].([]map[string]any)
	if len(rootItems) != 1 {
		t.Fatalf("expected one series card, got %#v", rootItems)
	}
	seriesID := rootItems[0]["Id"].(string)
	if rootItems[0]["Type"] != "Series" || rootItems[0]["IsFolder"] != true || rootItems[0]["Name"] != "间谍过家家" {
		t.Fatalf("unexpected series payload: %#v", rootItems[0])
	}

	seasons, err := svc.Items(t.Context(), ItemsParams{ParentID: seriesID, Limit: 50})
	if err != nil {
		t.Fatalf("series items: %v", err)
	}
	seasonItems := seasons["Items"].([]map[string]any)
	if len(seasonItems) != 1 || seasonItems[0]["Type"] != "Season" || seasonItems[0]["IndexNumber"] != 2 {
		t.Fatalf("unexpected seasons: %#v", seasonItems)
	}

	episodes, err := svc.Items(t.Context(), ItemsParams{ParentID: seasonItems[0]["Id"].(string), IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("season episodes: %v", err)
	}
	episodeItems := episodes["Items"].([]map[string]any)
	if len(episodeItems) != 2 || episodeItems[0]["Type"] != "Episode" || episodeItems[0]["Name"] != "第 1 集" {
		t.Fatalf("unexpected episodes: %#v", episodeItems)
	}
	if episodeItems[0]["SeriesId"] != seriesID || episodeItems[0]["ParentId"] != seasonItems[0]["Id"] {
		t.Fatalf("episode hierarchy not linked: %#v", episodeItems[0])
	}

	latest, err := svc.LatestItems(t.Context(), "user-1", lib.ID, 10)
	if err != nil {
		t.Fatalf("latest items: %v", err)
	}
	if len(latest) != 1 || latest[0]["Type"] != "Series" {
		t.Fatalf("latest should be grouped by series: %#v", latest)
	}

	playback, err := svc.PlaybackInfo(t.Context(), seriesID, "user-1")
	if err != nil {
		t.Fatalf("series playback fallback: %v", err)
	}
	sources := playback["MediaSources"].([]map[string]any)
	if sources[0]["Id"] != "ep-1" {
		t.Fatalf("series playback should fall back to first episode: %#v", sources)
	}
	if sources[0]["DirectStreamUrl"] != "/Videos/ep-1/stream.mkv" {
		t.Fatalf("playback should use Emby-compatible stream URL: %#v", sources[0])
	}
}

func TestEmbySeriesGroupingPaginatesAfterFullLibraryGrouping(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "国漫", Path: `/media/anime`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	rows := make([]model.Media, 0, 25*40)
	for series := 1; series <= 25; series++ {
		for episode := 1; episode <= 40; episode++ {
			created := now.Add(time.Duration(series*1000+episode) * time.Second)
			rows = append(rows, model.Media{
				Base:       model.Base{ID: fmt.Sprintf("show-%02d-ep-%02d", series, episode), CreatedAt: created, UpdatedAt: created},
				LibraryID:  lib.ID,
				Title:      fmt.Sprintf("测试番 %02d", series),
				Path:       fmt.Sprintf(`/media/anime/测试番 %02d/Season 01/测试番 %02d.S01E%02d.mkv`, series, series, episode),
				SeasonNum:  1,
				EpisodeNum: episode,
			})
		}
	}
	if err := svc.repo.DB.CreateInBatches(rows, 200).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 20})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	if root["TotalRecordCount"] != 25 {
		t.Fatalf("series total = %#v, want 25", root["TotalRecordCount"])
	}
	rootItems := root["Items"].([]map[string]any)
	if len(rootItems) != 20 {
		t.Fatalf("first page series len = %d, want 20", len(rootItems))
	}
	if rootItems[0]["RecursiveItemCount"] != 40 {
		t.Fatalf("first series episode count = %#v, want 40", rootItems[0]["RecursiveItemCount"])
	}

	latest, err := svc.LatestItems(t.Context(), "user-1", lib.ID, 25)
	if err != nil {
		t.Fatalf("latest items: %v", err)
	}
	if len(latest) != 25 {
		t.Fatalf("latest series len = %d, want 25", len(latest))
	}

	counts, err := svc.ItemCounts(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("item counts: %v", err)
	}
	if counts["SeriesCount"] != 25 || counts["EpisodeCount"] != int64(1000) {
		t.Fatalf("counts = %#v, want 25 series and 1000 episodes", counts)
	}
}

func TestEmbyItemsKeepSpecialsInSeasonZero(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "番剧", Path: `F:\downloads\日番`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:       model.Base{ID: "sp-1"},
		LibraryID:  lib.ID,
		Title:      "间谍过家家",
		Path:       `F:\downloads\日番\间谍过家家\Specials\间谍过家家 - S00E01.mkv`,
		PosterURL:  `F:\episode-still.jpg`,
		SeasonNum:  0,
		EpisodeNum: 1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	rootItems := root["Items"].([]map[string]any)
	if len(rootItems) != 1 || rootItems[0]["Type"] != "Series" {
		t.Fatalf("expected one series card, got %#v", rootItems)
	}

	seasons, err := svc.Items(t.Context(), ItemsParams{ParentID: rootItems[0]["Id"].(string), Limit: 50})
	if err != nil {
		t.Fatalf("series seasons: %v", err)
	}
	seasonItems := seasons["Items"].([]map[string]any)
	if len(seasonItems) != 1 || seasonItems[0]["Type"] != "Season" || seasonItems[0]["IndexNumber"] != 0 || seasonItems[0]["Name"] != "特别篇" {
		t.Fatalf("specials should be exposed as season zero: %#v", seasonItems)
	}

	episodes, err := svc.Items(t.Context(), ItemsParams{ParentID: seasonItems[0]["Id"].(string), IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("special episodes: %v", err)
	}
	episodeItems := episodes["Items"].([]map[string]any)
	if len(episodeItems) != 1 {
		t.Fatalf("expected one special episode, got %#v", episodeItems)
	}
	if episodeItems[0]["ParentIndexNumber"] != 0 || episodeItems[0]["SeasonId"] != seasonItems[0]["Id"] || episodeItems[0]["ParentId"] != seasonItems[0]["Id"] {
		t.Fatalf("special episode linked to wrong season: %#v season=%#v", episodeItems[0], seasonItems[0])
	}
	if tags, ok := episodeItems[0]["ImageTags"].(map[string]string); !ok || tags["Primary"] != "sp-1" {
		t.Fatalf("episode still should be exposed as Primary image: %#v", episodeItems[0]["ImageTags"])
	}
}

func TestEmbyEpisodeStillIsPrimaryImageNotArt(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "剧集", Path: `/media/tv`, Type: "tv", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:        model.Base{ID: "ep-still"},
		LibraryID:   lib.ID,
		Title:       "间谍过家家",
		Path:        `/media/tv/间谍过家家/Season 02/间谍过家家 - S02E01.mkv`,
		PosterURL:   `https://image.example/show-poster.jpg`,
		BackdropURL: `https://image.example/episode-still.jpg`,
		SeasonNum:   2,
		EpisodeNum:  1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	item := svc.itemPayload(t.Context(), &media, false, 0)
	if tags, ok := item["ImageTags"].(map[string]string); !ok || tags["Primary"] != "ep-still" {
		t.Fatalf("episode should expose a primary image tag: %#v", item["ImageTags"])
	}
	if tags, ok := item["BackdropImageTags"].([]string); !ok || len(tags) != 0 {
		t.Fatalf("episode still must not be exposed as art/backdrop: %#v", item["BackdropImageTags"])
	}
	primary, err := svc.ImageURL(t.Context(), "ep-still", "Primary")
	if err != nil {
		t.Fatalf("primary image url: %v", err)
	}
	if primary != media.BackdropURL {
		t.Fatalf("episode Primary image = %q, want still %q", primary, media.BackdropURL)
	}
	art, err := svc.ImageURL(t.Context(), "ep-still", "Art")
	if err != nil {
		t.Fatalf("art image url: %v", err)
	}
	if art == media.BackdropURL {
		t.Fatalf("episode still must not be returned as Art image")
	}
}

func TestEmbyVirtualSeriesArtworkUsesListCache(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "番剧", Path: `/media/anime`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:        model.Base{ID: "ep-1"},
		LibraryID:   lib.ID,
		Title:       "剑来",
		Path:        `/media/anime/剑来/Season 01/剑来 - S01E01.mkv`,
		PosterURL:   `/poster.jpg`,
		BackdropURL: `/backdrop.jpg`,
		SeasonNum:   1,
		EpisodeNum:  1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	seriesID := items[0]["Id"].(string)

	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	poster, err := svc.ImageURL(cancelled, seriesID, "Primary")
	if err != nil {
		t.Fatalf("image url from cache: %v", err)
	}
	if poster != "/poster.jpg" {
		t.Fatalf("poster = %q, want cached poster", poster)
	}
	backdrop, err := svc.ImageURL(cancelled, seriesID, "Backdrop")
	if err != nil {
		t.Fatalf("backdrop url from cache: %v", err)
	}
	if backdrop != "/backdrop.jpg" {
		t.Fatalf("backdrop = %q, want cached backdrop", backdrop)
	}
}

func TestEmbyCloudAnimeUsesSeriesNameFromChineseSeasonFolder(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "OpenList · 国漫", Path: `cloud://openlist/国漫`, Type: "anime", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	for _, media := range []model.Media{
		{
			Base:       model.Base{ID: "cloud-ep-1"},
			LibraryID:  lib.ID,
			Title:      "04",
			Path:       `cloud://openlist/国漫/剑来/第二季/04.mkv`,
			SeasonNum:  2,
			EpisodeNum: 4,
		},
		{
			Base:       model.Base{ID: "cloud-ep-2"},
			LibraryID:  lib.ID,
			Title:      "05",
			Path:       `cloud://openlist/国漫/剑来/第二季/05.mkv`,
			SeasonNum:  2,
			EpisodeNum: 5,
		},
	} {
		if err := svc.repo.DB.Create(&media).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Type"] != "Series" || items[0]["Name"] != "剑来" {
		t.Fatalf("cloud anime should be grouped as one series named 剑来, got %#v", items)
	}
}
