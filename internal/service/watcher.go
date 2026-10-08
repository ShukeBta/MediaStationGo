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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
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

	mu             sync.Mutex
	watcher        *fsnotify.Watcher
	watched        map[string]string       // dir -> libraryID
	pending        map[string]pendingEvent // path -> most recent change
	stop           chan struct{}
	stopOnce       sync.Once
	refreshMu      sync.Mutex
	progress       func(found, watched int)
	onRefresh      func(error)
	refreshRetryAt time.Time
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

func (w *WatcherService) setRefreshCallback(callback func(error)) {
	w.refreshMu.Lock()
	defer w.refreshMu.Unlock()
	w.onRefresh = callback
}

// Start initialises the underlying fsnotify watcher and registers every
// library root currently in the database.
func (w *WatcherService) Start(ctx context.Context) error {
	w.mu.Lock()
	select {
	case <-w.stop:
		w.mu.Unlock()
		return errors.New("watcher stopped")
	default:
	}
	if w.watcher != nil {
		w.mu.Unlock()
		return nil
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		w.mu.Unlock()
		return err
	}
	w.watcher = fw
	w.mu.Unlock()
	go w.loop(ctx)
	go w.debouncer(ctx)
	err = w.Refresh(ctx)
	if err != nil {
		w.log.Warn("watcher refresh failed", zap.Error(err))
	}
	return err
}

// Stop tears down the watcher (called on graceful shutdown).
func (w *WatcherService) Stop() {
	w.stopOnce.Do(func() {
		close(w.stop)
		w.mu.Lock()
		fw := w.watcher
		w.mu.Unlock()
		if fw != nil {
			_ = fw.Close()
		}
	})
}

// Refresh reads the library list and adjusts the set of watched
// directories. Idempotent — safe to call after every CRUD.
func (w *WatcherService) Refresh(ctx context.Context) (refreshErr error) {
	w.refreshMu.Lock()
	defer w.refreshMu.Unlock()
	defer func() {
		if ctx.Err() != nil {
			return
		}
		select {
		case <-w.stop:
			return
		default:
		}
		w.mu.Lock()
		if w.watcher == nil {
			w.mu.Unlock()
			return
		}
		wasRetrying := !w.refreshRetryAt.IsZero()
		if refreshErr != nil {
			w.refreshRetryAt = time.Now().Add(30 * time.Second)
		} else {
			w.refreshRetryAt = time.Time{}
		}
		w.mu.Unlock()
		if w.onRefresh != nil {
			w.onRefresh(refreshErr)
		}
		if wasRetrying && refreshErr == nil {
			w.log.Info("watcher refresh recovered")
		}
	}()
	libs, err := w.repo.Library.List(ctx)
	if err != nil {
		w.log.Warn("watcher library list failed", zap.Error(err))
		return err
	}
	w.mu.Lock()
	if w.watcher == nil {
		w.mu.Unlock()
		return nil
	}
	previous := make(map[string]string, len(w.watched))
	for path, id := range w.watched {
		previous[path] = id
	}
	w.mu.Unlock()

	// Map every directory (root + all subdirectories) to its library so new
	// files anywhere in the tree raise events — fsnotify itself is
	// non-recursive, so we register each directory explicitly.
	current := make(map[string]string)
	failedRoots := []string{}
	var failures []error
	report := func() {
		if w.progress == nil {
			return
		}
		w.mu.Lock()
		count := len(w.watcher.WatchList())
		w.mu.Unlock()
		w.progress(len(current), count)
	}
	defer report()
	for _, l := range libs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !l.Enabled {
			continue
		}
		roots := l.Roots
		if len(roots) == 0 && l.Path != "" {
			roots = []model.LibraryRoot{{LibraryID: l.ID, Path: l.Path, Enabled: true}}
		}
		for _, root := range roots {
			if !root.Enabled {
				continue
			}
			if _, _, ok := parseCloudLibraryPath(root.Path); ok {
				continue
			}
			watchRoot, info, err := resolveAccessibleMappedPath(root.Path)
			if err != nil || !info.IsDir() {
				if err == nil {
					err = errors.New("watch path is not a directory")
				}
				w.log.Warn("watch path inaccessible",
					zap.String("path", root.Path),
					zap.String("library_id", l.ID),
					zap.String("root_id", root.ID),
					zap.Error(err))
				failedRoots = append(failedRoots, root.Path)
				for _, candidate := range mappedPathCandidates(root.Path) {
					failedRoots = append(failedRoots, candidate)
				}
				if err != nil {
					failures = append(failures, fmt.Errorf("watch root %q: %w", root.Path, err))
				}
				continue
			}
			err = walkDirsForWatch(ctx, watchRoot, func(dir string) {
				current[dir] = l.ID
				if len(current)%128 == 0 {
					report()
				}
			})
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				failedRoots = append(failedRoots, watchRoot)
				w.log.Warn("watch directory traversal failed", zap.String("path", watchRoot), zap.String("library_id", l.ID), zap.Error(err))
				failures = append(failures, fmt.Errorf("walk watch root %q: %w", watchRoot, err))
			}
		}
	}
	// Remove disappeared paths.
	w.mu.Lock()
	defer w.mu.Unlock()
	for path := range previous {
		if _, ok := current[path]; !ok {
			keep := false
			for _, root := range failedRoots {
				if sameLibraryPath(path, root) || pathWithin(path, root) {
					keep = true
					break
				}
			}
			if keep {
				continue
			}
			_ = w.watcher.Remove(path)
			delete(w.watched, path)
		}
	}
	// Add new ones.
	actual := map[string]bool{}
	for _, path := range w.watcher.WatchList() {
		actual[filepath.Clean(path)] = true
	}
	for path, id := range current {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, ok := w.watched[path]; ok && actual[filepath.Clean(path)] {
			w.watched[path] = id
			continue
		}
		if err := w.watcher.Add(path); err != nil {
			w.log.Warn("watch add failed", zap.String("path", path), zap.Error(err))
			failures = append(failures, fmt.Errorf("watch add %q: %w", path, err))
			continue
		}
		w.watched[path] = id
		if w.progress != nil && len(w.watched)%128 == 0 {
			w.progress(len(current), len(w.watched))
		}
	}
	return errors.Join(failures...)
}

// watchDirRecursive registers a newly-created directory subtree so files
// copied into it afterwards still raise events.
func (w *WatcherService) watchDirRecursive(dir, libraryID string) {
	for _, d := range listDirsForWatch(dir) {
		if _, ok := w.watched[d]; ok {
			continue
		}
		if err := w.watcher.Add(d); err != nil {
			w.log.Warn("watch add (recursive) failed", zap.String("path", d), zap.Error(err))
			w.refreshRetryAt = time.Now().Add(30 * time.Second)
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
		w.refreshIfDue(ctx, now)
		for _, d := range due {
			w.process(ctx, d)
		}
	}
}

func (w *WatcherService) refreshIfDue(ctx context.Context, now time.Time) {
	w.mu.Lock()
	due := !w.refreshRetryAt.IsZero() && !now.Before(w.refreshRetryAt)
	w.mu.Unlock()
	if due {
		_ = w.Refresh(ctx)
	}
}

// process ingests or removes a single changed path.
func (w *WatcherService) process(ctx context.Context, d duePath) {
	fi, err := os.Stat(d.path)
	if err != nil {
		// A detached disk or permission error is not a confirmed file deletion.
		if !os.IsNotExist(err) || !w.libraryRootAccessible(ctx, d.libraryID, d.path) {
			w.retryPath(d)
			return
		}
		// Vanished (delete/rename away): drop its media row if any.
		if removed, derr := w.scanner.RemovePath(ctx, d.path); derr != nil {
			w.log.Warn("watcher remove failed", zap.String("path", d.path), zap.Error(derr))
		} else if removed > 0 {
			w.log.Info("watcher removed media", zap.String("path", d.path))
		}
		return
	}
	if fi.IsDir() {
		// A moved-in directory can already contain files before watches attach.
		w.mu.Lock()
		w.watchDirRecursive(d.path, d.libraryID)
		w.mu.Unlock()
		_ = walkLocalMediaTree(d.path, func(path string, info walkInfo) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if info.isDir {
				return nil
			}
			_, err := w.scanner.IngestPath(ctx, d.libraryID, path)
			return err
		})
		return
	}
	if added, ierr := w.scanner.IngestPath(ctx, d.libraryID, d.path); ierr != nil {
		w.log.Warn("watcher ingest failed", zap.String("path", d.path), zap.Error(ierr))
	} else if added {
		w.log.Info("watcher ingested media", zap.String("path", d.path))
	}
}

func (w *WatcherService) retryPath(d duePath) {
	w.mu.Lock()
	if _, exists := w.pending[d.path]; !exists {
		w.pending[d.path] = pendingEvent{libraryID: d.libraryID, ts: time.Now().Add(25 * time.Second)}
	}
	w.mu.Unlock()
}

func (w *WatcherService) libraryRootAccessible(ctx context.Context, libraryID, path string) bool {
	lib, err := w.repo.Library.FindByID(ctx, libraryID)
	if err != nil || lib == nil {
		return false
	}
	root, err := w.scanner.localLibraryRootForPath(ctx, lib, path)
	if err != nil || root == nil {
		return false
	}
	watchRoot, info, err := resolveAccessibleMappedPath(root.Path)
	if err != nil || !info.IsDir() {
		return false
	}
	// A linked disk may disappear while the enclosing library stays online.
	// Preserve rows when an existing link in the logical path has no target.
	for current := filepath.Clean(path); sameLibraryPath(current, watchRoot) || pathWithin(current, watchRoot); current = filepath.Dir(current) {
		entry, statErr := os.Lstat(current)
		if statErr != nil && !os.IsNotExist(statErr) {
			return false
		}
		if statErr == nil && entry.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(current); err != nil {
				return false
			}
		}
		if sameLibraryPath(current, watchRoot) {
			break
		}
	}
	return true
}
