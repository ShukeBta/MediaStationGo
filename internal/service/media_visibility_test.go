package service

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/config"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"go.uber.org/zap"
)

func TestMediaVisibilityFiltersNSFWAndLibraries(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{}, &model.Setting{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	libA := model.Library{Name: "电影", Path: "/media/movies", Type: "movie", Enabled: true}
	libB := model.Library{Name: "成人", Path: "/media/adult", Type: "movie", Enabled: true}
	if err := db.Create(&libA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&libB).Error; err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), AdultLibraryIDsSettingKey, `["`+libB.ID+`"]`); err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: libA.ID, Title: "普通电影", Path: "/media/movies/a.mkv"},
		{LibraryID: libA.ID, Title: "成人电影", Path: "/media/movies/b.mkv", NSFW: true},
		{LibraryID: libB.ID, Title: "限制媒体库电影", Path: "/media/adult/c.mkv"},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	hiddenAdultLibraries := AdultLibraryIDs(t.Context(), repos)
	items, err := svc.SearchMediaVisible(t.Context(), "电影", 20, MediaVisibility{
		IncludeNSFW:      false,
		HiddenLibraryIDs: hiddenAdultLibraries,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sortedMediaTitles(items); !slices.Equal(got, []string{"普通电影"}) {
		t.Fatalf("NSFW-filtered search = %#v", got)
	}

	items, err = svc.SearchMediaVisible(t.Context(), "电影", 20, MediaVisibility{
		IncludeNSFW:       true,
		AllowedLibraryIDs: []string{libA.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := sortedMediaTitles(items); !slices.Equal(got, []string{"成人电影", "普通电影"}) {
		t.Fatalf("library-filtered search = %#v", got)
	}

	listed, total, err := svc.ListMediaVisible(t.Context(), libA.ID, 1, 20, MediaVisibility{
		IncludeNSFW:      false,
		HiddenLibraryIDs: hiddenAdultLibraries,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(listed) != 1 || listed[0].Title != "普通电影" {
		t.Fatalf("NSFW-filtered list total=%d rows=%#v", total, sortedMediaTitles(listed))
	}

	listed, total, err = svc.ListMediaVisible(t.Context(), libB.ID, 1, 20, MediaVisibility{
		IncludeNSFW:      false,
		HiddenLibraryIDs: hiddenAdultLibraries,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(listed) != 0 {
		t.Fatalf("adult library should be hidden total=%d rows=%#v", total, sortedMediaTitles(listed))
	}
}

func TestMediaVisibilityHidesDeprecatedNativeCloudLibraries(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)

	legacy := model.Library{
		Name:    "旧云盘",
		Path:    BuildCloudLibraryPath(LegacyQuarkProvider, "archive", "archive"),
		Type:    "movie",
		Enabled: true,
	}
	openList := model.Library{
		Name:    "OpenList",
		Path:    BuildCloudLibraryPath("openlist", "movies", "movies"),
		Type:    "movie",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &legacy); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &openList); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Media{
		{LibraryID: legacy.ID, Title: "历史媒体", Path: "cloud://" + LegacyQuarkProvider + "/archive/old.mkv"},
		{LibraryID: openList.ID, Title: "可见媒体", Path: "cloud://openlist/movies/new.mkv"},
	}).Error; err != nil {
		t.Fatal(err)
	}

	items, err := svc.SearchMediaVisible(t.Context(), "媒体", 20, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if got := sortedMediaTitles(items); !slices.Equal(got, []string{"可见媒体"}) {
		t.Fatalf("deprecated native cloud media should be hidden from search, got %#v", got)
	}

	listed, total, err := svc.ListMediaVisible(t.Context(), legacy.ID, 1, 20, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(listed) != 0 {
		t.Fatalf("deprecated native cloud media should be hidden from direct list total=%d rows=%#v", total, sortedMediaTitles(listed))
	}
}

func TestConfiguredAdultLibrariesDoNotHideSafeLibraryWithNSFWItems(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{})
	repos := repository.New(db)

	safe := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", Enabled: true}
	adult := model.Library{Name: "9KG", Path: "/media/9KG", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &safe); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &adult); err != nil {
		t.Fatal(err)
	}
	if err := repos.Setting.Set(t.Context(), AdultLibraryIDsSettingKey, `["`+adult.ID+`"]`); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Media{
		{LibraryID: safe.ID, Title: "普通电影", Path: "/media/movie/a.mkv"},
		{LibraryID: safe.ID, Title: "误入普通库的成人条目", Path: "/media/movie/b.mkv", NSFW: true},
		{LibraryID: adult.ID, Title: "成人影片", Path: "/media/9KG/c.mkv"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{Username: "viewer", PasswordHash: "hash", Role: "user", HideAdult: true}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}

	visibility := UserDefaultMediaVisibility(t.Context(), repos, viewer.ID)
	if LibraryVisibleForUser(t.Context(), repos, safe, visibility) != true {
		t.Fatal("configured adult libraries should not hide a safe library just because it contains NSFW items")
	}
	if LibraryVisibleForUser(t.Context(), repos, adult, visibility) != false {
		t.Fatal("configured adult library should be hidden when the user hides adult content")
	}

	items, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisible(t.Context(), "电影", 20, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if got := sortedMediaTitles(items); !slices.Equal(got, []string{"普通电影"}) {
		t.Fatalf("safe library should stay visible while NSFW media is filtered, got %#v", got)
	}
}

func TestSearchMediaVisibleHonorsLargePosterWallLimit(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "海报墙", Path: "/media/all", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	rows := make([]model.Media, 260)
	for i := range rows {
		rows[i] = model.Media{
			LibraryID: lib.ID,
			Title:     fmt.Sprintf("节目 %03d", i),
			Path:      fmt.Sprintf("/media/all/show-%03d.mkv", i),
		}
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	items, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisible(t.Context(), "", 240, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 240 {
		t.Fatalf("large poster wall search returned %d rows, want 240", len(items))
	}
}

func TestSearchMediaVisibleCanReturnHugeLibraryResultsWhenRequested(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "海量剧集", Path: "/media/huge", Type: "tv", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	const total = 2505
	rows := make([]model.Media, total)
	for i := range rows {
		rows[i] = model.Media{
			LibraryID:  lib.ID,
			Title:      fmt.Sprintf("海量剧集 %04d", i),
			Path:       fmt.Sprintf("/media/huge/show-%04d.mkv", i),
			SeasonNum:  1,
			EpisodeNum: i + 1,
		}
	}
	if err := db.CreateInBatches(&rows, 500).Error; err != nil {
		t.Fatal(err)
	}

	items, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisible(t.Context(), "海量剧集", total, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != total {
		t.Fatalf("huge search returned %d rows, want %d", len(items), total)
	}

	firstPage, totalRows, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisiblePage(t.Context(), "海量剧集", 1, 2000, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if totalRows != total || len(firstPage) != 2000 {
		t.Fatalf("huge search page 1 len=%d total=%d, want len=2000 total=%d", len(firstPage), totalRows, total)
	}
	secondPage, totalRows, err := NewMediaService(&config.Config{}, zap.NewNop(), repos).
		SearchMediaVisiblePage(t.Context(), "海量剧集", 2, 2000, MediaVisibility{IncludeNSFW: true})
	if err != nil {
		t.Fatal(err)
	}
	if totalRows != total || len(secondPage) != total-2000 {
		t.Fatalf("huge search page 2 len=%d total=%d, want len=%d total=%d", len(secondPage), totalRows, total-2000, total)
	}
}

func sortedMediaTitles(rows []model.Media) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Title)
	}
	slices.Sort(out)
	return out
}
