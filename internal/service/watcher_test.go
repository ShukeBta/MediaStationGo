package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
	"gorm.io/gorm"

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

func TestWatcherRetryRecoversMissingRootAndStartupWarning(t *testing.T) {
	base := t.TempDir()
	available, missing := filepath.Join(base, "available"), filepath.Join(base, "missing-mount")
	if err := os.Mkdir(available, 0o755); err != nil {
		t.Fatal(err)
	}
	db := newServiceTestDB(t, &model.Library{})
	repos := repository.New(db)
	for _, root := range []string{available, missing} {
		if err := repos.Library.Create(t.Context(), &model.Library{Name: filepath.Base(root), Path: root, Type: "movie", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fw.Close() })
	w := NewWatcherService(zap.NewNop(), repos, nil)
	w.watcher = fw
	c := &Container{Startup: NewStartupState()}
	w.progress = c.Startup.updateDirectories
	w.onRefresh = func(err error) { c.Startup.updateStageWarning("建立媒体库目录监听", err) }
	err = c.startupStep("建立媒体库目录监听", func() error { return w.Refresh(t.Context()) })
	if err == nil || !strings.Contains(err.Error(), missing) {
		t.Fatalf("missing root error must identify path: %v", err)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("watcher created missing mount: %v", err)
	}
	if len(fw.WatchList()) != 1 || w.refreshRetryAt.IsZero() {
		t.Fatalf("healthy root not retained or retry missing: %v", fw.WatchList())
	}
	c.Startup.updateStageWarning("其他初始化步骤", errors.New("still failing"))
	c.Startup.finish("ready")
	if got := c.StartupStatus(); len(got.Warnings) != 2 || got.DirectoriesWatched != 1 {
		t.Fatalf("partial failure: %+v", got)
	}
	// Retry while still missing must retain one warning, not report success.
	w.refreshIfDue(t.Context(), w.refreshRetryAt)
	if got := c.StartupStatus(); len(got.Warnings) != 2 {
		t.Fatalf("retry lost/duplicated warning: %+v", got)
	}
	if err := os.Mkdir(missing, 0o755); err != nil {
		t.Fatal(err)
	}
	w.refreshIfDue(t.Context(), w.refreshRetryAt)
	got := c.StartupStatus()
	if !w.refreshRetryAt.IsZero() || got.DirectoriesFound != 2 || got.DirectoriesWatched != 2 || len(got.Warnings) != 1 || !strings.HasPrefix(got.Warnings[0], "其他初始化步骤") {
		t.Fatalf("recovery status: %+v retry=%v", got, w.refreshRetryAt)
	}
	file := filepath.Join(missing, "new.mkv")
	if err := os.WriteFile(file, []byte("media"), 0o644); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-fw.Events:
		if event.Name != file {
			t.Fatalf("unexpected recovered event: %+v", event)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("recovered directory does not produce watch events")
	}
}

func TestWatcherRefreshDatabaseFailureSchedulesRetry(t *testing.T) {
	db := newServiceTestDB(t, &model.Library{})
	repos := repository.New(db)
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fw.Close() })
	w := NewWatcherService(zap.NewNop(), repos, nil)
	w.watcher = fw
	fail := true
	if err := db.Callback().Query().Before("gorm:query").Register("test:watcher_failure", func(tx *gorm.DB) {
		if fail {
			tx.AddError(errors.New("temporary library database failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := w.Refresh(t.Context()); err == nil || w.refreshRetryAt.IsZero() {
		t.Fatalf("query failure not retried: %v", err)
	}
	fail = false
	w.refreshIfDue(t.Context(), w.refreshRetryAt)
	if !w.refreshRetryAt.IsZero() {
		t.Fatal("database recovery did not clear retry")
	}
}
