package service

import (
	"slices"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func TestEmbyViewsMergeEpisodicCloudLibrariesIntoUserLibrary(t *testing.T) {
	svc := newTestEmbyService(t)
	local := model.Library{Name: "国漫", Path: "/media/动漫/国漫", Type: "tv", Enabled: true}
	cloud := model.Library{Name: "OpenList · 国漫", Path: BuildCloudLibraryPath("openlist", "/国漫", "/国漫"), Type: "anime", Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := svc.repo.Library.Create(t.Context(), lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}
	if err := svc.repo.DB.Create(&model.Media{
		Base:       model.Base{ID: "cloud-show-1"},
		LibraryID:  cloud.ID,
		Title:      "云盘国漫",
		Path:       "cloud://openlist/国漫/云盘国漫/Season 01/云盘国漫.S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	views, err := svc.Views(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	viewItems := views["Items"].([]map[string]any)
	if len(viewItems) != 1 {
		t.Fatalf("emby views = %#v, want one merged user-facing library", viewItems)
	}
	if viewItems[0]["Id"] != local.ID || viewItems[0]["Name"] != "国漫" {
		t.Fatalf("merged view should use local library identity, got %#v", viewItems[0])
	}

	items, err := svc.Items(t.Context(), ItemsParams{ParentID: local.ID, Recursive: true, IncludeItemTypes: []string{"Episode"}, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	episodes := items["Items"].([]map[string]any)
	if len(episodes) != 1 || episodes[0]["Id"] != "cloud-show-1" {
		t.Fatalf("merged local library should include cloud episodes, got %#v", episodes)
	}
}

func TestEmbyViewsMergeCloudCategoryAliasesIntoUserLibrary(t *testing.T) {
	svc := newTestEmbyService(t)
	local := model.Library{Name: "日番", Path: "/media/动漫/日番", Type: "tv", Enabled: true}
	cloud := model.Library{Name: "OpenList · 日漫", Path: BuildCloudLibraryPath("openlist", "/日漫", "/日漫"), Type: "anime", Enabled: true}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := svc.repo.Library.Create(t.Context(), lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}
	if err := svc.repo.DB.Create(&model.Media{
		Base:       model.Base{ID: "cloud-anime-1"},
		LibraryID:  cloud.ID,
		Title:      "云盘日漫",
		Path:       "cloud://openlist/日漫/云盘日漫/Season 01/云盘日漫.S01E01.mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	views, err := svc.Views(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	viewItems := views["Items"].([]map[string]any)
	if len(viewItems) != 1 || viewItems[0]["Id"] != local.ID || viewItems[0]["Name"] != "日番" {
		t.Fatalf("emby views = %#v, want cloud alias merged into local 日番", viewItems)
	}

	items, err := svc.Items(t.Context(), ItemsParams{ParentID: local.ID, Recursive: true, IncludeItemTypes: []string{"Episode"}, Limit: 50})
	if err != nil {
		t.Fatalf("items: %v", err)
	}
	episodes := items["Items"].([]map[string]any)
	if len(episodes) != 1 || episodes[0]["Id"] != "cloud-anime-1" {
		t.Fatalf("merged local 日番 library should include 日漫 cloud episodes, got %#v", episodes)
	}
}

func TestEmbyViewsExpandAllowedCloudAliasToMergedLocalLibrary(t *testing.T) {
	svc := newTestEmbyService(t)
	if err := svc.repo.DB.AutoMigrate(&model.PlayProfile{}); err != nil {
		t.Fatalf("migrate play profile: %v", err)
	}
	if err := svc.repo.User.Create(t.Context(), &model.User{
		Base:         model.Base{ID: "user-1"},
		Username:     "tester",
		PasswordHash: "x",
		Role:         "admin",
		Tier:         "plus",
		IsActive:     true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	local := model.Library{
		Base:    model.Base{ID: "local-anime"},
		Name:    "日番",
		Path:    "/media/动漫/日番",
		Type:    "tv",
		Enabled: true,
	}
	cloud := model.Library{
		Base:    model.Base{ID: "cloud-anime"},
		Name:    "OpenList · 日漫",
		Path:    BuildCloudLibraryPath("openlist", "/日漫", "/日漫"),
		Type:    "anime",
		Enabled: true,
	}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := svc.repo.Library.Create(t.Context(), lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}
	if err := svc.repo.PlayProfile.Create(t.Context(), &model.PlayProfile{
		UserID:            "user-1",
		Name:              "默认",
		IsDefault:         true,
		AllowAdult:        true,
		AllowedLibraryIDs: `["cloud-anime"]`,
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	visibility := svc.mediaVisibility(t.Context(), "user-1")
	if !slices.Contains(visibility.AllowedLibraryIDs, "local-anime") {
		t.Fatalf("allowed library ids = %#v, want merged local library", visibility.AllowedLibraryIDs)
	}
	libraries, err := svc.repo.Library.List(t.Context())
	if err != nil {
		t.Fatalf("list libraries: %v", err)
	}
	displayLibraries := FilterDisplayCloudLibraries(t.Context(), svc.repo, libraries)
	if len(displayLibraries) != 1 || displayLibraries[0].ID != "local-anime" {
		t.Fatalf("display libraries = %#v, want merged local library", displayLibraries)
	}
	if !svc.libraryVisibleFromCachedVisibility(displayLibraries[0], visibility) {
		t.Fatalf("merged display library should be visible with visibility %#v: %#v", visibility, displayLibraries[0])
	}

	views, err := svc.Views(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	items := views["Items"].([]map[string]any)
	if len(items) != 1 || items[0]["Id"] != "local-anime" || items[0]["Name"] != "日番" {
		t.Fatalf("emby views = %#v, want merged local 日番 view", items)
	}
}

func TestEmbyPlaybackAllowsCloudMediaMergedIntoAllowedLocalLibrary(t *testing.T) {
	svc := newTestEmbyService(t)
	if err := svc.repo.DB.AutoMigrate(&model.PlayProfile{}); err != nil {
		t.Fatalf("migrate play profile: %v", err)
	}
	local := model.Library{
		Base:    model.Base{ID: "local-anime"},
		Name:    "日番",
		Path:    "/media/动漫/日番",
		Type:    "tv",
		Enabled: true,
	}
	cloud := model.Library{
		Base:    model.Base{ID: "cloud-anime"},
		Name:    "OpenList · 日漫",
		Path:    BuildCloudLibraryPath("openlist", "/日漫", "/日漫"),
		Type:    "anime",
		Enabled: true,
	}
	for _, lib := range []*model.Library{&local, &cloud} {
		if err := svc.repo.Library.Create(t.Context(), lib); err != nil {
			t.Fatalf("create library: %v", err)
		}
	}
	if err := svc.repo.PlayProfile.Create(t.Context(), &model.PlayProfile{
		UserID:            "user-1",
		Name:              "默认",
		IsDefault:         true,
		AllowAdult:        true,
		AllowedLibraryIDs: `["local-anime"]`,
	}); err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if err := svc.repo.DB.Create(&model.Media{
		Base:       model.Base{ID: "cloud-episode"},
		LibraryID:  cloud.ID,
		Title:      "云盘日漫",
		Path:       "cloud://openlist/日漫/云盘日漫/Season 01/云盘日漫.S01E01.mkv",
		STRMURL:    "/api/cloud/play/openlist?ref=%2F%E6%97%A5%E6%BC%AB%2F%E4%BA%91%E7%9B%98%E6%97%A5%E6%BC%AB%2FSeason+01%2F%E4%BA%91%E7%9B%98%E6%97%A5%E6%BC%AB.S01E01.mkv",
		Container:  "mkv",
		SeasonNum:  1,
		EpisodeNum: 1,
	}).Error; err != nil {
		t.Fatalf("create media: %v", err)
	}

	item, err := svc.Item(t.Context(), "cloud-episode", "user-1")
	if err != nil {
		t.Fatalf("item: %v", err)
	}
	if item == nil {
		t.Fatalf("merged cloud media should be visible through allowed local library")
	}
	playback, err := svc.PlaybackInfo(t.Context(), "cloud-episode", "user-1")
	if err != nil {
		t.Fatalf("playback: %v", err)
	}
	if playback == nil {
		t.Fatalf("merged cloud media should return playback info")
	}
}

func TestEmbyViewsSplitMixedLibraryIntoMovieAndShowVirtualViews(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "混合库", Path: "/media/mixed", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{
			Base:      model.Base{ID: "movie-1"},
			LibraryID: lib.ID,
			Title:     "普通电影",
			Path:      "/media/mixed/普通电影.2026.mkv",
		},
		{
			Base:       model.Base{ID: "episode-1"},
			LibraryID:  lib.ID,
			Title:      "混合剧集",
			Path:       "/media/mixed/国产剧/混合剧集/Season 01/混合剧集 - S01E01.mkv",
			SeasonNum:  1,
			EpisodeNum: 1,
		},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	views, err := svc.Views(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	items := views["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("mixed library should expose only movie and show virtual views, got %#v", items)
	}
	want := map[string]string{
		virtualLibraryID("movies", lib.ID): "movies",
		virtualLibraryID("shows", lib.ID):  "tvshows",
	}
	for _, item := range items {
		id, _ := item["Id"].(string)
		if want[id] == "" {
			t.Fatalf("unexpected view %#v in %#v", item, items)
		}
		if item["CollectionType"] != want[id] {
			t.Fatalf("view %s collection type = %v, want %s: %#v", id, item["CollectionType"], want[id], item)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing virtual views: %#v from %#v", want, items)
	}
}

func TestEmbyViewsSplitMixedLibraryByEpisodeFilenameSignal(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "混合库", Path: "/media/mixed", Type: "auto", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{
			Base:      model.Base{ID: "movie-1"},
			LibraryID: lib.ID,
			Title:     "普通电影",
			Path:      "/media/mixed/普通电影.2026.mkv",
		},
		{
			Base:       model.Base{ID: "episode-1"},
			LibraryID:  lib.ID,
			Title:      "文件名识别剧集",
			Path:       "/media/mixed/文件名识别剧集.S01E01.mkv",
			SeasonNum:  1,
			EpisodeNum: 1,
		},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	views, err := svc.Views(t.Context(), "user-1")
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	items := views["Items"].([]map[string]any)
	if len(items) != 2 {
		t.Fatalf("mixed library should split by filename episode signal, got %#v", items)
	}
	want := map[string]string{
		virtualLibraryID("movies", lib.ID): "movies",
		virtualLibraryID("shows", lib.ID):  "tvshows",
	}
	for _, item := range items {
		id, _ := item["Id"].(string)
		if want[id] == "" {
			t.Fatalf("unexpected view %#v in %#v", item, items)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("missing virtual views: %#v from %#v", want, items)
	}
}

func TestEmbyVirtualLibraryItemsFilterMoviesAndShows(t *testing.T) {
	svc := newTestEmbyService(t)
	lib := model.Library{Name: "混合库", Path: "/media/mixed", Type: "movie", Enabled: true}
	if err := svc.repo.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	rows := []model.Media{
		{
			Base:      model.Base{ID: "movie-1"},
			LibraryID: lib.ID,
			Title:     "普通电影",
			Path:      "/media/mixed/普通电影.2026.mkv",
		},
		{
			Base:       model.Base{ID: "episode-1"},
			LibraryID:  lib.ID,
			Title:      "混合剧集",
			Path:       "/media/mixed/国产剧/混合剧集/Season 01/混合剧集 - S01E01.mkv",
			SeasonNum:  1,
			EpisodeNum: 1,
		},
	}
	for i := range rows {
		if err := svc.repo.DB.Create(&rows[i]).Error; err != nil {
			t.Fatalf("create media: %v", err)
		}
	}

	movies, err := svc.Items(t.Context(), ItemsParams{ParentID: virtualLibraryID("movies", lib.ID), Limit: 50})
	if err != nil {
		t.Fatalf("movie virtual items: %v", err)
	}
	movieItems := movies["Items"].([]map[string]any)
	if len(movieItems) != 1 || movieItems[0]["Id"] != "movie-1" || movieItems[0]["Type"] != "Movie" {
		t.Fatalf("movie virtual view should contain only real movies, got %#v", movieItems)
	}

	shows, err := svc.Items(t.Context(), ItemsParams{ParentID: virtualLibraryID("shows", lib.ID), Limit: 50})
	if err != nil {
		t.Fatalf("show virtual items: %v", err)
	}
	showItems := shows["Items"].([]map[string]any)
	if len(showItems) != 1 || showItems[0]["Type"] != "Series" {
		t.Fatalf("show virtual view should contain grouped series, got %#v", showItems)
	}
}
