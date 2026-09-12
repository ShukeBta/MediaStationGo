package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// walkLocalMediaTree follows directory and file links for read-only scanning.
// Logical paths stay under the configured library root; resolved directory
// identities prevent cycles and repeated traversal through directory aliases.
// Organizing retains its separate walker because it can move source files.
func walkLocalMediaTree(root string, fn func(string, walkInfo) error) error {
	w := localMediaTreeWalker{fn: fn, seen: make(map[string]struct{})}
	if err := w.visit(filepath.Clean(root), true); err != nil {
		return err
	}
	return w.readErr
}

type localMediaTreeWalker struct {
	fn      func(string, walkInfo) error
	seen    map[string]struct{}
	readErr error
}

func (w *localMediaTreeWalker) recordReadError(path string, err error) {
	if w.readErr == nil {
		w.readErr = fmt.Errorf("cannot read media path %q (check permissions, link target and disk mounts): %w", path, err)
	}
}

func (w *localMediaTreeWalker) visit(path string, root bool) error {
	info, err := os.Stat(path)
	if err != nil {
		w.recordReadError(path, err)
		return nil
	}
	if !info.IsDir() {
		if !info.Mode().IsRegular() {
			return nil
		}
		return w.fn(path, walkInfo{size: info.Size()})
	}
	// A user-selected hidden root is valid; only hidden descendants are skipped.
	if !root && strings.HasPrefix(info.Name(), ".") {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		w.recordReadError(path, err)
		return nil
	}
	resolved, err = filepath.Abs(resolved)
	if err != nil {
		w.recordReadError(path, err)
		return nil
	}
	if runtime.GOOS == "windows" {
		resolved = strings.ToLower(resolved)
	}
	if _, visited := w.seen[resolved]; visited {
		return nil
	}
	w.seen[resolved] = struct{}{}
	if err := w.fn(path, walkInfo{isDir: true}); err != nil {
		if err == filepath.SkipDir {
			return nil
		}
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		w.recordReadError(path, err)
		return nil
	}
	for _, entry := range entries {
		if err := w.visit(filepath.Join(path, entry.Name()), false); err != nil {
			return err
		}
	}
	return nil
}
