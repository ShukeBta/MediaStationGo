package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// walkDirsForWatch reads directory entries without stat-ing ordinary files.
// Directory links retain their logical library paths and share cycle detection
// with the scanner's semantics; explicit hidden roots remain supported.
func walkDirsForWatch(ctx context.Context, root string, visit func(string)) error {
	seen := map[string]bool{}
	var failures []error
	var walkDir func(string, bool) error
	walkDir = func(path string, isRoot bool) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !isRoot && strings.HasPrefix(filepath.Base(path), ".") {
			return nil
		}
		info, err := os.Stat(path)
		if err != nil {
			failures = append(failures, fmt.Errorf("watch directory %q: %w", path, err))
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			failures = append(failures, err)
			return nil
		}
		real, err = filepath.Abs(real)
		if err != nil {
			return err
		}
		if runtime.GOOS == "windows" {
			real = strings.ToLower(real)
		}
		if seen[real] {
			return nil
		}
		seen[real] = true
		visit(path)
		entries, err := os.ReadDir(path)
		if err != nil {
			failures = append(failures, err)
			return nil
		}
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return err
			}
			if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
				continue
			}
			if err := walkDir(filepath.Join(path, entry.Name()), false); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walkDir(filepath.Clean(root), true); err != nil {
		return err
	}
	return errors.Join(failures...)
}

func listDirsForWatch(root string) []string {
	dirs := []string{}
	_ = walkDirsForWatch(context.Background(), root, func(path string) { dirs = append(dirs, path) })
	return dirs
}
