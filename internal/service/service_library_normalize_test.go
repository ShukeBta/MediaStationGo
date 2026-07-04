package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestNormalizeLocalLibraryPathsRewritesRelativeDockerMediaRoot(t *testing.T) {
	root := t.TempDir()
	containerRoot := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerRoot, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerRoot)

	db := newServiceTestDB(t, &model.Library{}, &model.LibraryRoot{}, &model.Media{})
	repos := repository.New(db)
	lib := model.Library{Name: "国产剧", Path: filepath.Join("media", "电视剧", "国产剧"), Type: "tv", Enabled: true}
	if err := repos.Library.CreateWithRoots(t.Context(), &lib, []model.LibraryRoot{{
		Name:    "国产剧",
		Path:    filepath.Join("media", "电视剧", "国产剧"),
		Enabled: true,
	}}); err != nil {
		t.Fatal(err)
	}

	svc := &Container{Repo: repos}
	if err := svc.NormalizeLocalLibraryPaths(t.Context()); err != nil {
		t.Fatalf("NormalizeLocalLibraryPaths() error = %v", err)
	}

	got, err := repos.Library.FindByID(t.Context(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Clean(containerLibrary)
	if got.Path != want {
		t.Fatalf("library path = %q, want %q", got.Path, want)
	}
	if len(got.Roots) != 1 || got.Roots[0].Path != want {
		t.Fatalf("library roots = %#v, want path %q", got.Roots, want)
	}
}

func TestNormalizePersistedLocalLibraryPathKeepsEmptyPathEmpty(t *testing.T) {
	if got := normalizePersistedLocalLibraryPath(""); got != "" {
		t.Fatalf("normalizePersistedLocalLibraryPath(empty) = %q, want empty", got)
	}
}
