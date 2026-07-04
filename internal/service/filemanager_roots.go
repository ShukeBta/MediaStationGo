package service

import (
	"context"
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
	for _, root := range roots {
		if pathWithin(path, root) {
			return true
		}
	}
	return false
}

func (s *FileManagerService) isAllowedRoot(path string, roots map[string]string) bool {
	path = filepath.Clean(path)
	for _, root := range roots {
		if strings.EqualFold(path, filepath.Clean(root)) {
			return true
		}
	}
	return false
}
