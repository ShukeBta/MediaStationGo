// Package service — filesystem watcher.
//
// WatcherService observes every enabled library root with fsnotify and
// debounces incoming events into incremental, per-file ingests. New / renamed
// files become Media rows; deletes remove them.
//
// 设计目标：只在「有新增/变更媒体」时增量入库，绝不因为单个文件变化就对整个
// 媒体库做全量重扫——全量重扫会反复读盘、损伤硬盘，也是用户明确要避免的。
// 因此 watcher 递归监听库内所有子目录，事件去抖后只处理具体变化的路径。
//
// The watcher runs in the background and is started after migrations
// complete. It survives library add / delete via Refresh().
package service

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// pendingEvent records the most recent change to a path and the library it
// belongs to, for debounced incremental processing.
type pendingEvent struct {
	libraryID string
	ts        time.Time
}

// WatcherService is a thin orchestrator on top of fsnotify.
type WatcherService struct {
	log     *zap.Logger
	repo    *repository.Container
	scanner *ScannerService

	mu      sync.Mutex
	watcher *fsnotify.Watcher
	watched map[string]string       // dir -> libraryID
	pending map[string]pendingEvent // path -> most recent change
	stop    chan struct{}
}

// NewWatcherService is the constructor.
func NewWatcherService(log *zap.Logger, repo *repository.Container, scanner *ScannerService) *WatcherService {
	return &WatcherService{
		log:     log,
		repo:    repo,
		scanner: scanner,
		watched: make(map[string]string),
		pending: make(map[string]pendingEvent),
		stop:    make(chan struct{}),
	}
}

// Start initialises the underlying fsnotify watcher and registers every
// library root currently in the database.
func (w *WatcherService) Start(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	w.watcher = fw
	if err := w.Refresh(ctx); err != nil {
		w.log.Warn("watcher refresh failed", zap.Error(err))
	}
	go w.loop(ctx)
	go w.debouncer(ctx)
	return nil
}

// Stop tears down the watcher (called on graceful shutdown).
func (w *WatcherService) Stop() {
	close(w.stop)
	if w.watcher != nil {
		_ = w.watcher.Close()
	}
}

// Refresh reads the library list and adjusts the set of watched
// directories. Idempotent — safe to call after every CRUD.
func (w *WatcherService) Refresh(ctx context.Context) error {
	libs, err := w.repo.Library.List(ctx)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	// Map every directory (root + all subdirectories) to its library so new
	// files anywhere in the tree raise events — fsnotify itself is
	// non-recursive, so we register each directory explicitly.
	current := make(map[string]string)
	for _, l := range libs {
		if !l.Enabled {
			continue
		}
		for _, dir := range listDirsForWatch(l.Path) {
			current[dir] = l.ID
		}
	}
	// Remove disappeared paths.
	for path := range w.watched {
		if _, ok := current[path]; !ok {
			_ = w.watcher.Remove(path)
			delete(w.watched, path)
		}
	}
	// Add new ones.
	for path, id := range current {
		if _, ok := w.watched[path]; ok {
			continue
		}
		if err := w.watcher.Add(path); err != nil {
			w.log.Warn("watch add failed", zap.String("path", path), zap.Error(err))
			continue
		}
		w.watched[path] = id
	}
	return nil
}

// listDirsForWatch returns root plus every (non-hidden) subdirectory so the
// watcher can register the whole tree recursively.
func listDirsForWatch(root string) []string {
	dirs := []string{root}
	_ = walk(root, func(path string, info walkInfo) error {
		if info.isDir && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	return dirs
}

// watchDirRecursive registers a newly-created directory subtree so files
// copied into it afterwards still raise events.
func (w *WatcherService) watchDirRecursive(dir, libraryID string) {
	for _, d := range listDirsForWatch(dir) {
		if _, ok := w.watched[d]; ok {
			continue
		}
		if err := w.watcher.Add(d); err != nil {
			w.log.Debug("watch add (recursive) failed", zap.String("path", d), zap.Error(err))
			continue
		}
		w.watched[d] = libraryID
	}
}

// loop drains fsnotify events and pushes the affected library into the
// pending map. The actual rescan happens in the debouncer goroutine.
func (w *WatcherService) loop(ctx context.Context) {
	if w.watcher == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case ev, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename|fsnotify.Write) == 0 {
				continue
			}
			lib := w.findLibrary(ev.Name)
			if lib == "" {
				continue
			}
			// 新建目录：立即递归纳入监听，确保随后拷入的文件也能触发事件。
			if ev.Op&fsnotify.Create != 0 {
				if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
					w.mu.Lock()
					w.watchDirRecursive(ev.Name, lib)
					w.mu.Unlock()
				}
			}
			w.mu.Lock()
			w.pending[ev.Name] = pendingEvent{libraryID: lib, ts: time.Now()}
			w.mu.Unlock()
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.log.Warn("watcher error", zap.Error(err))
		}
	}
}

// findLibrary maps a path back to the watching library ID, taking the
// shortest matching prefix.
func (w *WatcherService) findLibrary(path string) string {
	w.mu.Lock()
	defer w.mu.Unlock()
	dir := filepath.Dir(path)
	for {
		if id, ok := w.watched[dir]; ok {
			return id
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// duePath couples a settled path with its library for incremental processing.
type duePath struct {
	path      string
	libraryID string
}

// debouncer drains the pending set every 5 s and processes each settled path
// incrementally: existing files are ingested (single-file upsert), vanished
// files are removed. Coalescing by path avoids storming the disk during bulk
// operations (mass-rename, large copies), and crucially we never re-walk the
// entire library — only the paths that actually changed.
func (w *WatcherService) debouncer(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stop:
			return
		case <-t.C:
		}
		w.mu.Lock()
		due := make([]duePath, 0, len(w.pending))
		now := time.Now()
		for path, ev := range w.pending {
			if now.Sub(ev.ts) >= 5*time.Second {
				due = append(due, duePath{path: path, libraryID: ev.libraryID})
				delete(w.pending, path)
			}
		}
		w.mu.Unlock()
		for _, d := range due {
			w.process(ctx, d)
		}
	}
}

// process ingests or removes a single changed path.
func (w *WatcherService) process(ctx context.Context, d duePath) {
	fi, err := os.Stat(d.path)
	if err != nil {
		// Vanished (delete/rename away): drop its media row if any.
		if removed, derr := w.scanner.RemovePath(ctx, d.path); derr != nil {
			w.log.Warn("watcher remove failed", zap.String("path", d.path), zap.Error(derr))
		} else if removed > 0 {
			w.log.Info("watcher removed media", zap.String("path", d.path))
		}
		return
	}
	if fi.IsDir() {
		return // directory events only matter for registering new watches
	}
	if added, ierr := w.scanner.IngestPath(ctx, d.libraryID, d.path); ierr != nil {
		w.log.Warn("watcher ingest failed", zap.String("path", d.path), zap.Error(ierr))
	} else if added {
		w.log.Info("watcher ingested media", zap.String("path", d.path))
	}
}
