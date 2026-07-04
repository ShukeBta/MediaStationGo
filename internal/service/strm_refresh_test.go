package service

import (
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestFindSTRMRefreshTargetsMatchesNestedLocalRoots(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{})
	repos := repository.New(db)
	base := t.TempDir()
	movieRoot := filepath.Join(base, "strm", "电影")
	tvRoot := filepath.Join(base, "strm", "电视剧")
	otherRoot := filepath.Join(base, "other")
	cloudRoot := "cloud://openlist/电影"

	movie := model.Library{Name: "电影 STRM", Path: movieRoot, Type: "movie", Enabled: true}
	tv := model.Library{Name: "电视剧 STRM", Path: tvRoot, Type: "tv", Enabled: true}
	other := model.Library{Name: "其他", Path: otherRoot, Type: "movie", Enabled: true}
	cloud := model.Library{Name: "云盘", Path: cloudRoot, Type: "movie", Enabled: true}
	disabled := model.Library{Name: "停用", Path: filepath.Join(base, "strm", "动漫"), Type: "tv", Enabled: false}
	for _, lib := range []*model.Library{&movie, &tv, &other, &cloud, &disabled} {
		if err := repos.Library.Create(t.Context(), lib); err != nil {
			t.Fatal(err)
		}
	}
	if err := repos.DB.Model(&model.Library{}).Where("id = ?", disabled.ID).Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}

	targets, err := FindSTRMRefreshTargets(t.Context(), repos, filepath.Join(base, "strm"))
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 {
		t.Fatalf("targets = %#v, want movie and tv only", targets)
	}
	got := map[string]bool{}
	for _, target := range targets {
		got[target.LibraryID] = true
	}
	if !got[movie.ID] || !got[tv.ID] || got[other.ID] || got[cloud.ID] || got[disabled.ID] {
		t.Fatalf("target libraries = %#v", targets)
	}
}

func TestFindSTRMRefreshTargetsDoesNotFallbackToAllLibraries(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{})
	repos := repository.New(db)
	lib := model.Library{Name: "电影", Path: filepath.Join(t.TempDir(), "movies"), Type: "movie", Enabled: true}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatal(err)
	}

	targets, err := FindSTRMRefreshTargets(t.Context(), repos, filepath.Join(t.TempDir(), "strm"))
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 0 {
		t.Fatalf("targets = %#v, want no fallback target", targets)
	}
}
