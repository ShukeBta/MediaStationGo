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
	libB := model.Library{Name: "成人", Path: "/media/adult", Type: "adult", Enabled: true}
	if err := db.Create(&libA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&libB).Error; err != nil {
		t.Fatal(err)
	}
	rows := []model.Media{
		{LibraryID: libA.ID, Title: "普通电影", Path: "/media/movies/a.mkv"},
		{LibraryID: libA.ID, Title: "成人电影", Path: "/media/movies/b.mkv", NSFW: true},
		{LibraryID: libB.ID, Title: "限制媒体库电影", Path: "/media/adult/c.mkv", NSFW: true},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}

	items, err := svc.SearchMediaVisible(t.Context(), "电影", 20, MediaVisibility{
		IncludeNSFW: false,
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
		IncludeNSFW: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(listed) != 1 || listed[0].Title != "普通电影" {
		t.Fatalf("NSFW-filtered list total=%d rows=%#v", total, sortedMediaTitles(listed))
	}

	listed, total, err = svc.ListMediaVisible(t.Context(), libB.ID, 1, 20, MediaVisibility{
		IncludeNSFW: false,
	})
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 || len(listed) != 0 {
		t.Fatalf("adult library should be hidden total=%d rows=%#v", total, sortedMediaTitles(listed))
	}
}

func TestDisabledLibraryIsNotVisibleToUsers(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{})
	repos := repository.New(db)
	lib := model.Library{Name: "停用库", Path: "/media/disabled", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.UpdateEnabled(t.Context(), lib.ID, false); err != nil {
		t.Fatal(err)
	}
	lib.Enabled = false
	if LibraryVisibleForUser(t.Context(), repos, lib, MediaVisibility{IncludeNSFW: true}) {
		t.Fatal("disabled library must not be visible to normal users")
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

func TestExplicitAdultLibraryDoesNotHideSafeLibraryWithNSFWItems(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{})
	repos := repository.New(db)

	safe := model.Library{Name: "电影", Path: "/media/movie", Type: "movie", Enabled: true}
	adult := model.Library{Name: "9KG", Path: "/media/9KG", Type: "adult", Enabled: true}
	if err := repos.Library.Create(t.Context(), &safe); err != nil {
		t.Fatal(err)
	}
	if err := repos.Library.Create(t.Context(), &adult); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Media{
		{LibraryID: safe.ID, Title: "普通电影", Path: "/media/movie/a.mkv"},
		{LibraryID: safe.ID, Title: "误入普通库的成人条目", Path: "/media/movie/b.mkv", NSFW: true},
		{LibraryID: adult.ID, Title: "成人影片", Path: "/media/9KG/c.mkv", NSFW: true},
	}).Error; err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{
		Username: "viewer", PasswordHash: "hash", Role: "user", HideAdult: true,
		AllowedLibraryIDs: []string{safe.ID, adult.ID},
	}
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

func TestLibraryIsAdultRequiresExplicitType(t *testing.T) {
	if !LibraryIsAdult(model.Library{Type: "adult"}) {
		t.Fatal("explicit adult library type should be recognised")
	}
	for _, lib := range []model.Library{
		{Name: "成人", Path: "/media/adult", Type: "movie"},
		{Name: "JAV", Path: "/media/jav", Type: "tv"},
	} {
		if LibraryIsAdult(lib) {
			t.Fatalf("library name/path must not imply adult type: %#v", lib)
		}
	}
}

func TestUserLibraryAccessIsHardLimitForDefaultProfile(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{})
	repos := repository.New(db)

	libraries := []model.Library{
		{Base: model.Base{ID: "library-a"}, Name: "A", Path: "/media/a", Type: "movie", Enabled: true},
		{Base: model.Base{ID: "library-b"}, Name: "B", Path: "/media/b", Type: "movie", Enabled: true},
		{Base: model.Base{ID: "library-c"}, Name: "C", Path: "/media/c", Type: "movie", Enabled: true},
	}
	if err := db.Create(&libraries).Error; err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{
		Base:              model.Base{ID: "viewer-limited"},
		Username:          "viewer-limited",
		PasswordHash:      "hash",
		Role:              "user",
		AllowedLibraryIDs: []string{"library-a", "library-b"},
	}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	profile := &model.PlayProfile{
		UserID:            viewer.ID,
		Name:              "默认",
		IsDefault:         true,
		AllowAdult:        true,
		AllowedLibraryIDs: `["library-b","library-c"]`,
	}
	if err := db.Create(profile).Error; err != nil {
		t.Fatal(err)
	}

	visibility := UserDefaultMediaVisibility(t.Context(), repos, viewer.ID)
	if !visibility.Allows(&model.Media{LibraryID: "library-b"}) {
		t.Fatal("profile should retain the library also granted by the administrator")
	}
	for _, id := range []string{"library-a", "library-c"} {
		if visibility.Allows(&model.Media{LibraryID: id}) {
			t.Fatalf("library %s should be outside the effective intersection", id)
		}
	}

	denied := CombineAllowedLibraryIDs(t.Context(), repos, []string{"library-a"}, []string{"library-c"})
	deniedVisibility := MediaVisibility{IncludeNSFW: true, AllowedLibraryIDs: denied}
	if deniedVisibility.Allows(&model.Media{LibraryID: "library-b"}) {
		t.Fatal("disjoint administrator/profile scopes must deny all libraries")
	}
}

func TestUnassignedUserCannotSeeAnyLibraryAndProfileCannotExpandScope(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{})
	repos := repository.New(db)
	lib := model.Library{Base: model.Base{ID: "library-unassigned"}, Name: "未分配库", Path: "/media/unassigned", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{
		Base:         model.Base{ID: "viewer-unassigned"},
		Username:     "viewer-unassigned",
		PasswordHash: "hash",
		Role:         "user",
	}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PlayProfile{
		UserID: viewer.ID, Name: "尝试扩权", IsDefault: true, AllowAdult: true, AllowedLibraryIDs: `["library-unassigned"]`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Media{LibraryID: lib.ID, Title: "未分配作品", Path: "/media/unassigned/movie.mkv"}).Error; err != nil {
		t.Fatal(err)
	}

	visibility := UserDefaultMediaVisibility(t.Context(), repos, viewer.ID)
	if visibility.Allows(&model.Media{LibraryID: lib.ID}) {
		t.Fatal("an unassigned user must not gain library access from a play profile")
	}
	if LibraryVisibleForUser(t.Context(), repos, lib, visibility) {
		t.Fatal("an unassigned library card must stay hidden")
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	items, err := svc.SearchMediaVisible(t.Context(), "", 20, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("unassigned search leaked media: %#v", items)
	}
	recent, err := svc.ListRecentSeriesCards(t.Context(), 20, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 0 {
		t.Fatalf("unassigned recent additions leaked media: %#v", recent)
	}
}

func TestAdministratorWithoutAssignmentsRemainsUnrestricted(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Setting{}, &model.PlayProfile{})
	repos := repository.New(db)
	admin := &model.User{Base: model.Base{ID: "admin"}, Username: "admin", PasswordHash: "hash", Role: "admin"}
	if err := repos.User.Create(t.Context(), admin); err != nil {
		t.Fatal(err)
	}
	visibility := UserDefaultMediaVisibility(t.Context(), repos, admin.ID)
	if !visibility.Allows(&model.Media{LibraryID: "any-library"}) {
		t.Fatal("administrators must remain unrestricted")
	}
}

func TestAdminBlockedUserFiltersAdultWorksInsideMixedLibrary(t *testing.T) {
	db := newServiceTestDB(t, &model.User{}, &model.Library{}, &model.Media{}, &model.Setting{}, &model.PlayProfile{})
	repos := repository.New(db)
	lib := model.Library{Name: "综合媒体库", Path: "/media/mixed", Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}
	viewer := &model.User{
		Base:                model.Base{ID: "blocked-viewer"},
		Username:            "blocked-viewer",
		PasswordHash:        "hash",
		Role:                "user",
		AllowedLibraryIDs:   []string{lib.ID},
		HideAdult:           false,
		AdultContentBlocked: true,
	}
	if err := repos.User.Create(t.Context(), viewer); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.PlayProfile{
		UserID: viewer.ID, Name: "允许成人的默认配置", IsDefault: true, AllowAdult: true,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.Media{
		{LibraryID: lib.ID, Title: "普通作品", Path: "/media/mixed/safe.mkv"},
		{LibraryID: lib.ID, Title: "成人作品", Path: "/media/mixed/adult.mkv", NSFW: true},
	}).Error; err != nil {
		t.Fatal(err)
	}

	visibility := UserDefaultMediaVisibility(t.Context(), repos, viewer.ID)
	if visibility.IncludeNSFW {
		t.Fatal("administrator block must override user preference and an adult-enabled play profile")
	}
	if !LibraryVisibleForUser(t.Context(), repos, lib, visibility) {
		t.Fatal("a mixed library must remain visible when only its adult works are blocked")
	}
	svc := NewMediaService(&config.Config{}, zap.NewNop(), repos)
	items, err := svc.SearchMediaVisible(t.Context(), "作品", 20, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if got := sortedMediaTitles(items); !slices.Equal(got, []string{"普通作品"}) {
		t.Fatalf("blocked search leaked adult works: %#v", got)
	}
	works, total, err := svc.SearchMediaVisibleSeriesPage(t.Context(), "作品", 1, 20, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(works) != 1 || works[0].Rep.Title != "普通作品" {
		t.Fatalf("blocked work search leaked adult works: total=%d works=%#v", total, works)
	}
	recent, err := svc.ListRecentSeriesCards(t.Context(), 20, visibility)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Rep.Title != "普通作品" {
		t.Fatalf("blocked recent additions leaked adult works: %#v", recent)
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
	if err := db.CreateInBatches(&rows, 100).Error; err != nil {
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
