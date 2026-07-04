package service

import (
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
