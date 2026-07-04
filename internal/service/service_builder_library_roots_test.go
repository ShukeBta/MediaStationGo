package service

import (
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestServiceBuilderLibraryRootsIncludesAllEnabledRoots(t *testing.T) {
	rootA := t.TempDir()
	rootB := t.TempDir()
	rootDisabled := t.TempDir()
	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{})
	repos := repository.New(db)
	lib := &model.Library{Name: "电影", Path: rootA, Type: "movie", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), lib, []model.LibraryRoot{
		{Name: "硬盘1", Path: rootA, Enabled: true},
		{Name: "硬盘2", Path: rootB, Enabled: true, SortOrder: 1},
		{Name: "离线", Path: rootDisabled, Enabled: false, SortOrder: 2},
	}); err != nil {
		t.Fatal(err)
	}
	disabledOnly := &model.Library{Name: "禁用库", Path: rootDisabled, Type: "movie", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), disabledOnly, []model.LibraryRoot{
		{Name: "离线", Path: rootDisabled, Enabled: false},
	}); err != nil {
		t.Fatal(err)
	}

	got := (&serviceContainerBuilder{repos: repos}).libraryRoots()
	want := map[string]bool{
		filepath.Clean(rootA): true,
		filepath.Clean(rootB): true,
	}
	for _, path := range got {
		delete(want, filepath.Clean(path))
		if filepath.Clean(path) == filepath.Clean(rootDisabled) {
			t.Fatalf("disabled root was returned: %#v", got)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing enabled roots %v from %#v", want, got)
	}
}
