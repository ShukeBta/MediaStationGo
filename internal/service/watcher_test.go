package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func TestWatcherRefreshMapsHostLibraryPathToContainerPath(t *testing.T) {
	root := t.TempDir()
	hostMedia := filepath.Join(root, "nas-host", "media")
	containerMedia := filepath.Join(root, "container", "media")
	containerLibrary := filepath.Join(containerMedia, "电视剧", "国产剧")
	if err := os.MkdirAll(containerLibrary, 0o755); err != nil {
		t.Fatalf("mkdir container library: %v", err)
	}
	t.Setenv("MEDIASTATION_MEDIA_DIR", hostMedia)
	t.Setenv("MEDIASTATION_MEDIA_CONTAINER_DIR", containerMedia)

	db := newServiceTestDB(t, &model.Library{})
	repos := repository.New(db)
	lib := model.Library{
		Base:    model.Base{ID: "lib-tv"},
		Name:    "国产剧",
		Path:    filepath.Join(hostMedia, "电视剧", "国产剧"),
		Type:    "tv",
		Enabled: true,
	}
	if err := repos.Library.Create(t.Context(), &lib); err != nil {
		t.Fatalf("create library: %v", err)
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("new watcher: %v", err)
	}
	defer fw.Close()
	watcher := NewWatcherService(zap.NewNop(), repos, nil)
	watcher.watcher = fw

	if err := watcher.Refresh(t.Context()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if _, ok := watcher.watched[filepath.Clean(containerLibrary)]; !ok {
		t.Fatalf("expected mapped container path watched, got %#v", watcher.watched)
	}
	if _, ok := watcher.watched[filepath.Clean(lib.Path)]; ok {
		t.Fatalf("host path should not be watched inside container: %#v", watcher.watched)
	}
}
