package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
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
			OriginalName: "第 1 集",
			Path:         `F:\downloads\日番\剧集\间谍过家家\Season 02\间谍过家家 - S02E01.mkv`,
			PosterURL:    `F:\poster.jpg`,
			SeasonNum:    2,
			EpisodeNum:   1,
		},
		{
			Base:         model.Base{ID: "ep-2"},
			LibraryID:    lib.ID,
			Title:        "间谍过家家",
			OriginalName: "第 2 集",
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

func TestEmbyVirtualSeriesArtworkFallsBackToLocalEpisodeThumbnail(t *testing.T) {
	svc := newTestEmbyService(t)
	workDir := t.TempDir()
	svc.cfg.Cache.CacheDir = filepath.Join(workDir, "cache")
	ffmpegPath := filepath.Join(workDir, "ffmpeg")
	if err := os.WriteFile(ffmpegPath, []byte("#!/bin/sh\nlast=\nfor arg do\n  last=$arg\ndone\nprintf '\\377\\330\\377\\331' > \"$last\"\n"), 0o755); err != nil {
		t.Fatalf("write fake ffmpeg: %v", err)
	}
	svc.cfg.App.FFmpegPath = ffmpegPath

	videoPath := filepath.Join(workDir, "shows", "胆胆", "Season 01", "胆胆 - S01E01.mp4")
	if err := os.MkdirAll(filepath.Dir(videoPath), 0o755); err != nil {
		t.Fatalf("mkdir video dir: %v", err)
	}
	if err := os.WriteFile(videoPath, []byte("fake video"), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}

	lib := model.Library{Name: "本地", Path: filepath.Join(workDir, "shows"), Type: "auto", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:       model.Base{ID: "local-ep-1"},
		LibraryID:  lib.ID,
		Title:      "胆胆",
		Path:       videoPath,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("library items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Type"] != "Series" {
		t.Fatalf("expected local episode to be grouped as a series, got %#v", items)
	}
	seriesID := items[0]["Id"].(string)
	seriesTags := items[0]["ImageTags"].(map[string]string)
	if seriesTags["Primary"] != seriesID {
		t.Fatalf("series should advertise primary image fallback, got %#v", seriesTags)
	}

	seriesThumb, err := svc.ImageURL(t.Context(), seriesID, "Primary")
	if err != nil {
		t.Fatalf("series image fallback: %v", err)
	}
	if stat, err := os.Stat(seriesThumb); err != nil || stat.Size() == 0 {
		t.Fatalf("series thumbnail not generated at %q stat=%#v err=%v", seriesThumb, stat, err)
	}

	seasons, err := svc.Items(t.Context(), ItemsParams{ParentID: seriesID, Limit: 50})
	if err != nil {
		t.Fatalf("series items: %v", err)
	}
	seasonItems := seasons["Items"].([]map[string]any)
	if len(seasonItems) != 1 || seasonItems[0]["Type"] != "Season" {
		t.Fatalf("expected one season, got %#v", seasonItems)
	}
	seasonID := seasonItems[0]["Id"].(string)
	seasonTags := seasonItems[0]["ImageTags"].(map[string]string)
	if seasonTags["Primary"] != seasonID {
		t.Fatalf("season should advertise primary image fallback, got %#v", seasonTags)
	}
	seasonThumb, err := svc.ImageURL(t.Context(), seasonID, "Primary")
	if err != nil {
		t.Fatalf("season image fallback: %v", err)
	}
	if seasonThumb != seriesThumb {
		t.Fatalf("season should reuse first episode thumbnail, got %q want %q", seasonThumb, seriesThumb)
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

func TestEmbyMovieTypedLibraryAutoDetectsEpisodes(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "顶级媒体", Path: `/media`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	movie := model.Media{
		Base:      model.Base{ID: "movie-1"},
		LibraryID: lib.ID,
		Title:     "真正的电影",
		Path:      `/media/Movie.2024.mkv`,
		PosterURL: `/poster-movie.jpg`,
	}
	episode := model.Media{
		Base:       model.Base{ID: "episode-1"},
		LibraryID:  lib.ID,
		Title:      "自动识别的剧",
		Path:       `/media/Shows/自动识别的剧/Season 01/自动识别的剧.S01E01.mkv`,
		PosterURL:  `/poster-episode.jpg`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	if err := svc.repo.DB.Create(&movie).Error; err != nil {
		t.Fatalf("create movie: %v", err)
	}
	if err := svc.repo.DB.Create(&episode).Error; err != nil {
		t.Fatalf("create episode: %v", err)
	}

	view := svc.libraryAsView(t.Context(), &lib)
	if view["CollectionType"] != "mixed" {
		t.Fatalf("mixed library collection type = %#v", view["CollectionType"])
	}
	if view["RecursiveItemCount"] != 2 || view["ChildCount"] != 2 {
		t.Fatalf("mixed library counts = recursive %#v child %#v", view["RecursiveItemCount"], view["ChildCount"])
	}
	views, err := svc.Views(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	viewItems := views["Items"].([]map[string]any)
	if len(viewItems) != 3 {
		t.Fatalf("mixed library should expose real view plus movie/show virtual views, got %#v", viewItems)
	}
	if viewItems[0]["Id"] != lib.ID || viewItems[0]["CollectionType"] != "mixed" {
		t.Fatalf("mixed library view should use real library id/type, got %#v", viewItems[0])
	}
	var movieViewID, showViewID string
	for _, item := range viewItems[1:] {
		switch item["CollectionType"] {
		case "movies":
			movieViewID = item["Id"].(string)
			if item["RecursiveItemCount"] != 1 || item["ChildCount"] != 1 {
				t.Fatalf("movie virtual view counts = %#v", item)
			}
		case "tvshows":
			showViewID = item["Id"].(string)
			if item["RecursiveItemCount"] != 1 || item["ChildCount"] != 1 {
				t.Fatalf("show virtual view counts = %#v", item)
			}
		}
	}
	if movieViewID == "" || showViewID == "" {
		t.Fatalf("split views should expose movies and tvshows, got %#v", viewItems)
	}
	counts, err := svc.ItemsCounts(t.Context(), "user-1", "")
	if err != nil {
		t.Fatalf("item counts: %v", err)
	}
	if counts["MovieCount"] != 1 || counts["SeriesCount"] != 1 || counts["EpisodeCount"] != 1 || counts["ItemCount"] != 2 {
		t.Fatalf("unexpected item counts: %#v", counts)
	}

	movieViewItems, err := svc.Items(t.Context(), ItemsParams{ParentID: movieViewID, IncludeItemTypes: []string{"Movie"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("movie virtual view items: %v", err)
	}
	movieOnly := movieViewItems["Items"].([]map[string]any)
	if len(movieOnly) != 1 || movieOnly[0]["Id"] != movie.ID || movieOnly[0]["Type"] != "Movie" {
		t.Fatalf("movie virtual view should only expose movie rows, got %#v", movieViewItems)
	}
	showViewItems, err := svc.Items(t.Context(), ItemsParams{ParentID: showViewID, IncludeItemTypes: []string{"Series"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("show virtual view items: %v", err)
	}
	showOnly := showViewItems["Items"].([]map[string]any)
	if len(showOnly) != 1 || showOnly[0]["Type"] != "Series" {
		t.Fatalf("show virtual view should expose series rows, got %#v", showViewItems)
	}
	movieLatest, err := svc.LatestItems(t.Context(), "user-1", movieViewID, 10)
	if err != nil {
		t.Fatalf("movie virtual latest: %v", err)
	}
	if len(movieLatest) != 1 || movieLatest[0]["Id"] != movie.ID || movieLatest[0]["Type"] != "Movie" {
		t.Fatalf("movie virtual latest should expose movie rows, got %#v", movieLatest)
	}
	showLatest, err := svc.LatestItems(t.Context(), "user-1", showViewID, 10)
	if err != nil {
		t.Fatalf("show virtual latest: %v", err)
	}
	if len(showLatest) != 1 || showLatest[0]["Type"] != "Series" {
		t.Fatalf("show virtual latest should expose series rows, got %#v", showLatest)
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("mixed library should expose movie and series, got %#v", out)
	}
	types := map[any]bool{}
	for _, item := range items {
		types[item["Type"]] = true
	}
	if !types["Movie"] || !types["Series"] {
		t.Fatalf("mixed library should include Movie and Series, got %#v", items)
	}

	episodes, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Episode"}, Limit: 50})
	if err != nil {
		t.Fatalf("episode query: %v", err)
	}
	episodeItems := episodes["Items"].([]map[string]any)
	if len(episodeItems) != 1 || episodeItems[0]["Type"] != "Episode" {
		t.Fatalf("movie-typed library should expose strong episode as Episode, got %#v", episodes)
	}

	item, err := svc.Item(t.Context(), episode.ID, "user-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item["Type"] != "Episode" {
		t.Fatalf("direct item should be Episode, got %#v", item)
	}
}

func TestEmbyItemsHonorsLargeClientLimit(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := make([]model.Media, 0, 80)
	for i := 0; i < 80; i++ {
		rows = append(rows, model.Media{
			Base:      model.Base{ID: fmt.Sprintf("movie-%03d", i)},
			LibraryID: lib.ID,
			Title:     fmt.Sprintf("Movie %03d", i),
			Path:      fmt.Sprintf("/media/movies/Movie %03d.mkv", i),
			Container: "mkv",
		})
	}
	if err := svc.repo.DB.Create(&rows).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		ParentID:         lib.ID,
		IncludeItemTypes: []string{"Movie"},
		Recursive:        true,
		Limit:            1000,
	})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != len(rows) {
		t.Fatalf("large client limit should return all rows in one page, got %d of %d: %#v", len(items), len(rows), out)
	}
	if out["TotalRecordCount"] != len(rows) {
		t.Fatalf("unexpected total count: %#v", out)
	}
}

func TestEmbyWeakEpisodeNumbersStayMovies(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "动画电影", Path: `/media/movies/animation`, Type: "Movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:       model.Base{ID: "weak-number-movie"},
		LibraryID:  lib.ID,
		Title:      "TG - 30",
		Path:       `/media/movies/TG - 30.mkv`,
		PosterURL:  `/poster.jpg`,
		SeasonNum:  1,
		EpisodeNum: 30,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Movie"}, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Type"] != "Movie" {
		t.Fatalf("weak parsed numbers should stay Movie, got %#v", out)
	}

	episodes, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Episode"}, Limit: 50})
	if err != nil {
		t.Fatalf("episode query: %v", err)
	}
	if len(episodes["Items"].([]map[string]any)) != 0 {
		t.Fatalf("weak parsed numbers should not expose Episode, got %#v", episodes)
	}

	rootMovies, err := svc.Items(t.Context(), ItemsParams{IncludeItemTypes: []string{"Movie"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("root movie query: %v", err)
	}
	rootItems := rootMovies["Items"].([]map[string]any)
	if len(rootItems) != 1 || rootItems[0]["Id"] != media.ID || rootItems[0]["Type"] != "Movie" {
		t.Fatalf("root movie query should include movie-library item despite season numbers, got %#v", rootItems)
	}

	rootEpisodes, err := svc.Items(t.Context(), ItemsParams{IncludeItemTypes: []string{"Episode"}, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("root episode query: %v", err)
	}
	if len(rootEpisodes["Items"].([]map[string]any)) != 0 {
		t.Fatalf("root episode query should not expose movie-library item, got %#v", rootEpisodes)
	}
}

func TestEmbyMovieLibraryFiltersMisplacedSeriesPaths(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	show := model.Media{
		Base:       model.Base{ID: "misplaced-show"},
		LibraryID:  lib.ID,
		Title:      "错放剧集",
		Path:       `/media/movies/国产剧/错放剧集/Season 01/错放剧集 - S01E01.mkv`,
		SeasonNum:  1,
		EpisodeNum: 1,
	}
	movie := model.Media{
		Base:      model.Base{ID: "movie"},
		LibraryID: lib.ID,
		Title:     "普通电影",
		Path:      `/media/movies/普通电影.2026.mkv`,
	}
	if err := svc.repo.DB.Create(&show).Error; err != nil {
		t.Fatalf("create show: %v", err)
	}
	if err := svc.repo.DB.Create(&movie).Error; err != nil {
		t.Fatalf("create movie: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, IncludeItemTypes: []string{"Movie"}, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != movie.ID {
		t.Fatalf("movie library should filter misplaced series paths, got %#v", items)
	}

	item, err := svc.Item(t.Context(), show.ID, "user-1")
	if err != nil {
		t.Fatalf("direct item: %v", err)
	}
	if item["Type"] != "Episode" {
		t.Fatalf("misplaced series path should be typed as Episode directly, got %#v", item)
	}
}

// TestEmbyMovieLibraryGroupsEpisodicContentIntoSeries 验证方案 B: 电影类型库里
// 混入的「剧集结构」内容(如整合成剧集的剧场版/合集动画, 路径含 Season/SxxE)
// 在常规浏览(未指定 IncludeItemTypes)时应聚成 Series 卡片, 而不是以散装单集
// (Episode)漏出; 同库里真正的电影仍按 Movie 显示。两类并存于同一电影库视图。
func TestEmbyMovieLibraryGroupsEpisodicContentIntoSeries(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "动画电影", Path: `/media/动画电影`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	base := time.Now()
	// 剧集结构内容: 同一部「高达剧场版」的两集, 单集 tmdb 各不相同(模拟 NFO 单集 id 污染)。
	rows := []model.Media{
		{
			Base:       model.Base{ID: "gundam-e13", CreatedAt: base.Add(2 * time.Minute)},
			LibraryID:  lib.ID,
			Title:      "高达剧场版",
			Path:       `/media/动画电影/高达剧场版/Season 01/高达剧场版 - S01E13.mkv`,
			PosterURL:  `/poster.jpg`,
			TMDbID:     4375419,
			SeasonNum:  1,
			EpisodeNum: 13,
		},
		{
			Base:       model.Base{ID: "gundam-e14", CreatedAt: base.Add(3 * time.Minute)},
			LibraryID:  lib.ID,
			Title:      "高达剧场版",
			Path:       `/media/动画电影/高达剧场版/Season 01/高达剧场版 - S01E14.mkv`,
			PosterURL:  `/poster.jpg`,
			TMDbID:     4375461,
			SeasonNum:  1,
			EpisodeNum: 14,
		},
		// 真正的电影。
		{
			Base:      model.Base{ID: "real-movie", CreatedAt: base.Add(1 * time.Minute)},
			LibraryID: lib.ID,
			Title:     "普通动画电影",
			Path:      `/media/动画电影/普通动画电影.2024.mkv`,
			PosterURL: `/poster.jpg`,
		},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	out, err := svc.Items(t.Context(), ItemsParams{ParentID: lib.ID, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	items := out["Items"].([]map[string]any)

	var seriesCount, movieCount, episodeCount int
	var seriesPayload map[string]any
	for _, item := range items {
		switch item["Type"] {
		case "Series":
			seriesCount++
			seriesPayload = item
		case "Movie":
			movieCount++
		case "Episode":
			episodeCount++
		}
	}
	if episodeCount != 0 {
		t.Fatalf("movie library must not leak flat episodes, got %d episode items: %#v", episodeCount, items)
	}
	if seriesCount != 1 {
		t.Fatalf("episodic content should collapse into exactly one Series card, got %d: %#v", seriesCount, items)
	}
	if movieCount != 1 {
		t.Fatalf("real movie should stay as one Movie item, got %d: %#v", movieCount, items)
	}
	// 两集 tmdb 不同, 但按路径剧名聚成同一 Series, 集数应为 2。
	if got := seriesPayload["RecursiveItemCount"]; got != 2 {
		t.Fatalf("series should contain both episodes despite differing tmdb ids, got RecursiveItemCount=%v", got)
	}

	// Series 卡片可下钻: 解析其 season → episodes。
	seriesID, _ := seriesPayload["Id"].(string)
	if seriesID == "" {
		t.Fatalf("series payload missing Id: %#v", seriesPayload)
	}
	drill, err := svc.Items(t.Context(), ItemsParams{ParentID: seriesID, Recursive: true, Limit: 50})
	if err != nil {
		t.Fatalf("series drill-down: %v", err)
	}
	drillItems := drill["Items"].([]map[string]any)
	if len(drillItems) != 2 {
		t.Fatalf("series drill-down should list both episodes, got %#v", drillItems)
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

func TestEmbyRootItemsExposeLibraries(t *testing.T) {
	svc := newTestEmbyService(t)
	for _, lib := range []model.Library{
		{Name: "电影", Path: `F:\downloads\电影`, Type: "movie", Enabled: true},
		{Name: "综艺", Path: `F:\downloads\综艺`, Type: "variety", Enabled: true},
	} {
		if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}

	root, err := svc.Items(t.Context(), ItemsParams{Limit: 50})
	if err != nil {
		t.Fatalf("root items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("expected root items to expose libraries, got %#v", items)
	}
	if items[0]["Type"] != "CollectionFolder" || items[1]["Type"] != "CollectionFolder" {
		t.Fatalf("root should return collection folders: %#v", items)
	}
	if items[1]["CollectionType"] != "tvshows" {
		t.Fatalf("variety libraries should use tvshows collection type: %#v", items[1])
	}
}

func TestEmbyFolderItemQueryExposesLibrariesForHome(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{Base: model.Base{ID: "movie-1"}, LibraryID: lib.ID, Title: "不应出现在文件夹查询", Path: `/media/movies/a.mkv`}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		IncludeItemTypes: []string{"Folder", "CollectionFolder"},
		Limit:            50,
	})
	if err != nil {
		t.Fatalf("folder items: %v", err)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 {
		t.Fatalf("expected one library folder, got %#v", items)
	}
	if items[0]["Type"] != "CollectionFolder" || items[0]["IsFolder"] != true {
		t.Fatalf("folder query should return collection folders, got %#v", items[0])
	}
}

func TestEmbyUnsupportedItemTypesDoNotLeakAllMedia(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{Base: model.Base{ID: "movie-1"}, LibraryID: lib.ID, Title: "普通电影", Path: `/media/movies/a.mkv`}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	for _, includeType := range []string{"BoxSet", "Game", "Book", "Audio", "MusicAlbum", "Playlist", "TvChannel"} {
		out, err := svc.Items(t.Context(), ItemsParams{
			IncludeItemTypes: []string{includeType},
			Recursive:        true,
			Limit:            50,
		})
		if err != nil {
			t.Fatalf("%s items: %v", includeType, err)
		}
		if out["TotalRecordCount"] != int64(0) {
			t.Fatalf("%s should not return media rows, got %#v", includeType, out)
		}
		items := out["Items"].([]map[string]any)
		if len(items) != 0 {
			t.Fatalf("%s should return an empty list, got %#v", includeType, items)
		}
	}
}

func TestEmbyItemsFiltersFavorites(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	favorite := model.Media{Base: model.Base{ID: "fav-1"}, LibraryID: lib.ID, Title: "收藏电影", Path: `/media/movies/fav.mkv`}
	normal := model.Media{Base: model.Base{ID: "normal-1"}, LibraryID: lib.ID, Title: "普通电影", Path: `/media/movies/normal.mkv`}
	if err := svc.repo.DB.Create(&favorite).Error; err != nil {
		t.Fatalf("create favorite media: %v", err)
	}
	if err := svc.repo.DB.Create(&normal).Error; err != nil {
		t.Fatalf("create normal media: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Favorite{UserID: viewer.ID, MediaID: favorite.ID}).Error; err != nil {
		t.Fatalf("create favorite: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		UserID:    viewer.ID,
		Filters:   []string{"IsFavorite"},
		Recursive: true,
		Limit:     50,
	})
	if err != nil {
		t.Fatalf("favorite items: %v", err)
	}
	if out["TotalRecordCount"] != int64(1) {
		t.Fatalf("expected one favorite, got %#v", out)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != favorite.ID {
		t.Fatalf("favorite filter returned wrong items: %#v", items)
	}
	userData := items[0]["UserData"].(map[string]any)
	if userData["IsFavorite"] != true {
		t.Fatalf("favorite payload should carry IsFavorite=true: %#v", userData)
	}
}

func TestEmbyItemsFiltersResumableForHome(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Base: model.Base{ID: "user-1"}, Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	resumable := model.Media{Base: model.Base{ID: "resume-1"}, LibraryID: lib.ID, Title: "继续观看", Path: `/media/movies/resume.mkv`, DurationSec: 120}
	normal := model.Media{Base: model.Base{ID: "normal-1"}, LibraryID: lib.ID, Title: "普通电影", Path: `/media/movies/normal.mkv`, DurationSec: 120}
	if err := svc.repo.DB.Create(&resumable).Error; err != nil {
		t.Fatalf("create resumable media: %v", err)
	}
	if err := svc.repo.DB.Create(&normal).Error; err != nil {
		t.Fatalf("create normal media: %v", err)
	}
	if err := svc.repo.DB.Create(&model.PlaybackHistory{
		UserID:     viewer.ID,
		MediaID:    resumable.ID,
		PositionMs: 30_000,
		DurationMs: 120_000,
		WatchedAt:  time.Now(),
		Completed:  false,
	}).Error; err != nil {
		t.Fatalf("create playback history: %v", err)
	}

	out, err := svc.Items(t.Context(), ItemsParams{
		UserID:     viewer.ID,
		Filters:    []string{"IsResumable"},
		Recursive:  true,
		SortBy:     "DatePlayed",
		SortOrder:  "Descending",
		Limit:      50,
		StartIndex: 0,
	})
	if err != nil {
		t.Fatalf("resumable items: %v", err)
	}
	if out["TotalRecordCount"] != int64(1) {
		t.Fatalf("expected one resumable item, got %#v", out)
	}
	items := out["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != resumable.ID {
		t.Fatalf("resumable filter returned wrong items: %#v", items)
	}
}

func TestEmbyUserPolicyDisablesDownloadsForViewers(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Username: "viewer", Role: "user", Tier: "free", IsActive: true}
	admin := &model.User{Username: "admin", Role: "admin", Tier: "plus", IsActive: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	if err := svc.repo.User.Create(t.Context(), admin); err != nil {
		t.Fatalf("create admin: %v", err)
	}

	viewerPayload, err := svc.FindUser(t.Context(), viewer.ID)
	if err != nil {
		t.Fatalf("viewer payload: %v", err)
	}
	adminPayload, err := svc.FindUser(t.Context(), admin.ID)
	if err != nil {
		t.Fatalf("admin payload: %v", err)
	}
	viewerPolicy := viewerPayload["Policy"].(map[string]any)
	adminPolicy := adminPayload["Policy"].(map[string]any)
	if viewerPolicy["EnableMediaPlayback"] != true {
		t.Fatalf("viewer must keep playback enabled: %#v", viewerPolicy)
	}
	if viewerPolicy["EnableContentDownloading"] != false ||
		viewerPolicy["EnableSyncTranscoding"] != false ||
		viewerPolicy["EnableMediaConversion"] != false {
		t.Fatalf("viewer must not be allowed to download/sync media: %#v", viewerPolicy)
	}
	if adminPolicy["EnableContentDownloading"] != true {
		t.Fatalf("admin should keep downloading capability: %#v", adminPolicy)
	}
}

func TestEmbyHidesAdultLibrariesForUserLock(t *testing.T) {
	svc := newTestEmbyService(t)
	viewer := &model.User{Username: "viewer", Role: "user", Tier: "free", IsActive: true, HideAdult: true}
	if err := svc.repo.User.Create(t.Context(), viewer); err != nil {
		t.Fatalf("create viewer: %v", err)
	}
	safe := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	adult := model.Library{Name: "9KG 成人", Path: `/media/9KG`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &safe); err != nil {
		t.Fatalf("create safe library: %v", err)
	}
	if err := svc.repo.Library.Create(t.Context(), &adult); err != nil {
		t.Fatalf("create adult library: %v", err)
	}
	if err := svc.repo.Setting.Set(t.Context(), AdultLibraryIDsSettingKey, `["`+adult.ID+`"]`); err != nil {
		t.Fatalf("set adult libraries: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{LibraryID: safe.ID, Title: "安全电影", Path: `/media/movies/a.mkv`}).Error; err != nil {
		t.Fatalf("create safe media: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{LibraryID: adult.ID, Title: "成人电影", Path: `/media/9KG/a.mkv`}).Error; err != nil {
		t.Fatalf("create adult media: %v", err)
	}

	root, err := svc.Items(t.Context(), ItemsParams{UserID: viewer.ID, Limit: 50})
	if err != nil {
		t.Fatalf("root items: %v", err)
	}
	items := root["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Name"] != "电影" {
		t.Fatalf("adult library should be hidden: %#v", items)
	}
	adultItems, err := svc.Items(t.Context(), ItemsParams{UserID: viewer.ID, ParentID: adult.ID, Limit: 50})
	if err != nil {
		t.Fatalf("adult items: %v", err)
	}
	if got := adultItems["TotalRecordCount"]; got != int64(0) {
		t.Fatalf("adult media should be hidden, total=%#v payload=%#v", got, adultItems)
	}
}

func TestEmbyPlaybackInfoRespectsDirectPlayOnly(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "电影", Path: `/media/movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{Base: model.Base{ID: "m-1"}, LibraryID: lib.ID, Title: "Inception", Path: `/media/movies/inception.mkv`}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	// 默认（关闭）：宿主机可转码，下发 TranscodingUrl。
	pb, err := svc.PlaybackInfo(t.Context(), "m-1", "user-1")
	if err != nil {
		t.Fatalf("playback info: %v", err)
	}
	src := pb["MediaSources"].([]map[string]any)[0]
	if src["SupportsTranscoding"] != true {
		t.Fatalf("expected SupportsTranscoding=true by default, got %#v", src["SupportsTranscoding"])
	}
	if _, ok := src["TranscodingUrl"]; !ok {
		t.Fatalf("expected TranscodingUrl present by default: %#v", src)
	}
	if src["TranscodingUrl"] != "/Videos/m-1/master.m3u8" {
		t.Fatalf("expected HLS TranscodingUrl by default, got %#v", src["TranscodingUrl"])
	}

	// 开启「客户端直连解码」：不再下发转码能力 / TranscodingUrl，仍保留 DirectStream。
	if err := svc.repo.Setting.Set(t.Context(), PlaybackDirectOnlySettingKey, "true"); err != nil {
		t.Fatalf("enable direct-only: %v", err)
	}
	pb, err = svc.PlaybackInfo(t.Context(), "m-1", "user-1")
	if err != nil {
		t.Fatalf("playback info (direct-only): %v", err)
	}
	src = pb["MediaSources"].([]map[string]any)[0]
	if src["SupportsTranscoding"] != false {
		t.Fatalf("expected SupportsTranscoding=false in direct-only mode, got %#v", src["SupportsTranscoding"])
	}
	if _, ok := src["TranscodingUrl"]; ok {
		t.Fatalf("expected no TranscodingUrl in direct-only mode: %#v", src)
	}
	if src["SupportsDirectPlay"] != true || src["DirectStreamUrl"] != "/Videos/m-1/stream.mkv" {
		t.Fatalf("direct-only must still allow direct play: %#v", src)
	}
}

func TestEmbyPlaybackInfoKeepsSTRMBehindStreamEndpoint(t *testing.T) {
	svc := newTestEmbyService(t)
	if err := svc.repo.Setting.Set(t.Context(), CloudPlaybackModeSettingKey, CloudPlaybackModeSTRM); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	lib := model.Library{Name: "夸克网盘", Path: `cloud://quark/0`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "cloud-1"},
		LibraryID: lib.ID,
		Title:     "Cloud Movie",
		Path:      `cloud://quark/f1`,
		STRMURL:   `/api/cloud/play/quark?ref=f1`,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	pb, err := svc.PlaybackInfo(t.Context(), "cloud-1", "user-1")
	if err != nil {
		t.Fatalf("playback info: %v", err)
	}
	src := pb["MediaSources"].([]map[string]any)[0]
	if src["IsRemote"] != true {
		t.Fatalf("strm media should be marked remote: %#v", src)
	}
	if src["DirectStreamUrl"] != "/api/stream/cloud-1" {
		t.Fatalf("strm playback should prefer /api/stream when enabled: %#v", src)
	}
	if src["Path"] != "/api/stream/cloud-1" {
		t.Fatalf("path should prefer /api/stream when enabled: %#v", src)
	}
	streams := src["MediaStreams"].([]map[string]any)
	if len(streams) == 0 || streams[0]["Type"] != "Video" {
		t.Fatalf("strm media should expose a fallback video stream for Android clients: %#v", src)
	}
}

func TestEmbyPlaybackInfoUsesVideoStreamWhenSTRMDisabled(t *testing.T) {
	svc := newTestEmbyService(t)
	if err := svc.repo.Setting.Set(t.Context(), CloudPlaybackModeSettingKey, CloudPlaybackModeRedirectProxy); err != nil {
		t.Fatalf("set cloud playback mode: %v", err)
	}
	lib := model.Library{Name: "OpenList", Path: `cloud://openlist/Movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "cloud-302"},
		LibraryID: lib.ID,
		Title:     "Cloud 302 Movie",
		Path:      `cloud://openlist/Movies/Movie`,
		STRMURL:   `/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv`,
		Container: "matroska,webm",
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	pb, err := svc.PlaybackInfo(t.Context(), "cloud-302", "user-1")
	if err != nil {
		t.Fatalf("playback info: %v", err)
	}
	src := pb["MediaSources"].([]map[string]any)[0]
	if src["DirectStreamUrl"] != "/Videos/cloud-302/stream.mkv" {
		t.Fatalf("302/proxy mode should use Emby video stream URL: %#v", src)
	}
	if src["Path"] != "/Videos/cloud-302/stream.mkv" {
		t.Fatalf("302/proxy mode path should use Emby video stream URL: %#v", src)
	}
}

func TestEmbyPlaybackInfoProbesMissingCloudTrackMetadata(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "OpenList", Path: `cloud://openlist/Movies`, Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	media := model.Media{
		Base:      model.Base{ID: "cloud-probe-1"},
		LibraryID: lib.ID,
		Title:     "云盘电影",
		Path:      `cloud://openlist/Movies/Movie.mkv`,
		STRMURL:   `http://nas.local/api/cloud/play/openlist?ref=%2FMovies%2FMovie.mkv`,
	}
	if err := svc.repo.DB.Create(&media).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}
	resolver := &fakeCloudPlaybackResolver{
		link: &cloud.DirectLink{
			URL:     "http://cdn.example.test/Movie.mkv",
			Headers: map[string]string{"Authorization": "Bearer probe-token"},
		},
	}
	prober := &fakeCloudPlaybackProber{
		probe: &ProbeResult{
			DurationSec: 3661,
			Width:       3840,
			Height:      2160,
			VideoCodec:  "hevc",
			AudioCodec:  "eac3",
			Container:   "matroska,webm",
		},
	}
	svc.SetCloudProbe(resolver, prober)

	if _, err := svc.PlaybackInfo(t.Context(), "cloud-probe-1", "user-1"); err != nil {
		t.Fatalf("playback info: %v", err)
	}

	// 探测现在是异步的（同步探测曾把起播拖慢最多 8 秒并放大云盘流量）。
	// 轮询等待后台探测结果落库。
	var persisted model.Media
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := svc.repo.DB.First(&persisted, "id = ?", "cloud-probe-1").Error; err != nil {
			t.Fatalf("reload media: %v", err)
		}
		if persisted.DurationSec > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if persisted.DurationSec != 3661 || persisted.Width != 3840 || persisted.Height != 2160 || persisted.VideoCodec != "hevc" || persisted.AudioCodec != "eac3" {
		t.Fatalf("probe metadata not persisted: %#v", persisted)
	}
	if resolver.typ != "openlist" || resolver.ref != "/Movies/Movie.mkv" {
		t.Fatalf("resolver called with typ=%q ref=%q", resolver.typ, resolver.ref)
	}
	if prober.rawURL != "http://cdn.example.test/Movie.mkv" || prober.headers["Authorization"] != "Bearer probe-token" {
		t.Fatalf("probe called with url=%q headers=%#v", prober.rawURL, prober.headers)
	}

	// 落库之后，再次请求 PlaybackInfo 应当带上完整轨道元数据。
	pb, err := svc.PlaybackInfo(t.Context(), "cloud-probe-1", "user-1")
	if err != nil {
		t.Fatalf("playback info (second): %v", err)
	}
	src := pb["MediaSources"].([]map[string]any)[0]
	if src["RunTimeTicks"] != int64(3661)*10_000_000 {
		t.Fatalf("runtime ticks not filled after async probe: %#v", src)
	}
	streams := src["MediaStreams"].([]map[string]any)
	if len(streams) != 2 || streams[0]["Codec"] != "hevc" || streams[1]["Codec"] != "eac3" {
		t.Fatalf("media streams not filled after async probe: %#v", streams)
	}
}

func newTestEmbyService(t *testing.T) *EmbyService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// 内存库 + 异步探测协程：限制为单连接，避免连接池新建连接时
	// 拿到一个空白的 :memory: 实例（no such table）。
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(&model.Library{}, &model.Series{}, &model.Media{}, &model.Favorite{}, &model.PlaybackHistory{}, &model.User{}, &model.Setting{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repos := repository.New(db)
	return NewEmbyService(&config.Config{}, zap.NewNop(), repos)
}

type fakeCloudPlaybackResolver struct {
	link *cloud.DirectLink
	typ  string
	ref  string
	ua   string
}

func (f *fakeCloudPlaybackResolver) CloudResolve(_ context.Context, typ, fileRef, clientUA string) (*cloud.DirectLink, error) {
	f.typ = typ
	f.ref = fileRef
	f.ua = clientUA
	return f.link, nil
}

type fakeCloudPlaybackProber struct {
	probe   *ProbeResult
	rawURL  string
	headers map[string]string
}

func (f *fakeCloudPlaybackProber) ProbeHTTP(_ context.Context, rawURL string, headers map[string]string) (*ProbeResult, error) {
	f.rawURL = rawURL
	f.headers = headers
	return f.probe, nil
}
