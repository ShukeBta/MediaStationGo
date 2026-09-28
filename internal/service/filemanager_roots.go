package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (s *FileManagerService) requireAllowedPath(path string, forbidRoot bool) (string, map[string]string, error) {
	roots, _, err := s.allowedRootList()
	if err != nil {
		return "", nil, err
	}
	abs, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil {
		return "", nil, err
	}
	if !s.withinAllowed(abs, roots) {
		return "", nil, ErrPathOutOfBounds
	}
	if forbidRoot && s.isAllowedRoot(abs, roots) {
		return "", nil, ErrRootMutation
	}
	return abs, roots, nil
}

// allowedRootList returns the union of configured storage roots as
// label → absolute-path plus a sorted UI list.
func (s *FileManagerService) allowedRootList() (map[string]string, []Root, error) {
	roots, err := s.allowedRoots()
	if err != nil {
		return nil, nil, err
	}
	rootList := make([]Root, 0, len(roots))
	seen := map[string]struct{}{}
	for label, p := range roots {
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		rootList = append(rootList, Root{Label: label, Path: p})
	}
	sort.Slice(rootList, func(i, j int) bool { return rootList[i].Label < rootList[j].Label })
	return roots, rootList, nil
}

func (s *FileManagerService) allowedRoots() (map[string]string, error) {
	roots := map[string]string{}
	add := func(label, p string) {
		if p == "" {
			return
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return
		}
		if _, err := os.Stat(abs); err != nil {
			return
		}
		roots[label] = abs
	}
	add("data", s.cfg.App.DataDir)
	add("cache", s.cfg.Cache.CacheDir)
	add("movies", s.cfg.Media.MoviesDir)
	add("tv", s.cfg.Media.TVDir)
	add("anime", s.cfg.Media.AnimeDir)
	add("downloads", envOrDefault("MEDIASTATION_DOWNLOAD_CONTAINER_DIR", "/downloads"))
	add("media", envOrDefault("MEDIASTATION_MEDIA_CONTAINER_DIR", "/media"))
	if s.repo != nil && s.repo.Setting != nil {
		addSetting := func(label, key string) {
			if value, err := s.repo.Setting.Get(context.Background(), key); err == nil {
				add(label, strings.TrimSpace(value))
			}
		}
		addSetting("organize-source", "organize.source_dir")
		addSetting("organize-target", "organize.target_dir")
		addSetting("qb-savepath", "qbittorrent.savepath")
	}
	if s.repo != nil && s.repo.Library != nil {
		libs, err := s.repo.Library.List(context.Background())
		if err == nil {
			for _, l := range libs {
				if len(l.Roots) > 0 {
					for i, root := range l.Roots {
						if !root.Enabled {
							continue
						}
						label := strings.TrimSpace(root.Name)
						if label == "" {
							label = fmt.Sprintf("路径%d", i+1)
						}
						add("library:"+l.Name+":"+label, resolveMappedDestinationPath(root.Path))
					}
					continue
				}
				add("library:"+l.Name, resolveMappedDestinationPath(l.Path))
			}
		}
	}
	return roots, nil
}

func (s *FileManagerService) withinAllowed(path string, roots map[string]string) bool {
	realPath, err := resolvedExistingAncestor(path)
	if err != nil {
		return false
	}
	for _, root := range roots {
		realRoot, err := filepath.EvalSymlinks(root)
		if err == nil && pathWithin(path, root) && pathWithin(realPath, realRoot) {
			return true
		}
	}
	return false
}

func (s *FileManagerService) isAllowedRoot(path string, roots map[string]string) bool {
	path = filepath.Clean(path)
	realPath, pathErr := resolvedExistingAncestor(path)
	for _, root := range roots {
		if strings.EqualFold(path, filepath.Clean(root)) {
			return true
		}
		realRoot, err := filepath.EvalSymlinks(root)
		if pathErr == nil && err == nil && sameFilePath(realPath, realRoot) {
			return true
		}
	}
	return false
}

// Resolve the existing prefix before appending missing components. EvalSymlinks
// handles Windows junctions as well as Unix links; a dangling link is not a
// missing ordinary directory and must not be accepted as a safe prefix.
func resolvedExistingAncestor(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	ancestor := abs
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			real, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(ancestor, abs)
			if err != nil {
				return "", err
			}
			return filepath.Join(real, rel), nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", os.ErrNotExist
		}
		ancestor = parent
	}
}

func sameFilePath(a, b string) bool {
	rel, err := filepath.Rel(a, b)
	return err == nil && rel == "."
}

// Open the configured root, not the user-selected parent. Root methods enforce
// the boundary again at the filesystem operation, even if a link is swapped
// after requireAllowedPath's friendly preflight checks.
func (s *FileManagerService) openAllowedRoot(path string, roots map[string]string) (*os.Root, string, error) {
	best := ""
	for _, root := range roots {
		if s.withinAllowed(path, map[string]string{"root": root}) && len(root) > len(best) {
			best = root
		}
	}
	if best == "" {
		return nil, "", ErrPathOutOfBounds
	}
	realRoot, err := filepath.EvalSymlinks(best)
	if err != nil {
		return nil, "", err
	}
	// Root rejects absolute symlinks even when they point back inside the
	// allowed tree. Resolve the parent to a root-relative path, leaving the
	// final component intact so deleting/renaming a link acts on that link.
	realParent, err := resolvedExistingAncestor(filepath.Dir(path))
	if err != nil {
		return nil, "", err
	}
	target := filepath.Join(realParent, filepath.Base(path))
	if !pathWithin(target, realRoot) {
		return nil, "", ErrPathOutOfBounds
	}
	rel, err := filepath.Rel(realRoot, target)
	if err != nil {
		return nil, "", err
	}
	root, err := os.OpenRoot(realRoot)
	return root, rel, err
}
