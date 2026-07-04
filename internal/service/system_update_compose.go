package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type systemUpdateComposeTarget struct {
	Dir     string
	File    string
	Command string
}

var systemUpdateComposeFiles = []string{
	"docker-compose.yml",
	"docker-compose.yaml",
	"compose.yml",
	"compose.yaml",
}

func (s *SystemUpdateService) resolveComposeTarget(ctx context.Context) (systemUpdateComposeTarget, string) {
	if target := s.composeTargetFromConfiguredDir(ctx); target.Dir != "" {
		return target, ""
	}
	if target := s.discoverComposeTarget(); target.Dir != "" {
		return target, ""
	}
	return systemUpdateComposeTarget{}, "未找到 docker-compose.yml / compose.yml，请在系统更新设置中填写 Docker Compose 安装目录"
}

func (s *SystemUpdateService) composeTargetFromConfiguredDir(ctx context.Context) systemUpdateComposeTarget {
	configured := s.setting(ctx, SystemUpdateComposeDirSettingKey, firstNonEmpty(os.Getenv("MEDIASTATION_UPDATE_COMPOSE_DIR"), os.Getenv("MEDIASTATION_UPDATE_WORKDIR")))
	return composeTargetInDir(configured)
}

func (s *SystemUpdateService) discoverComposeTarget() systemUpdateComposeTarget {
	for _, dir := range s.composeCandidateDirs() {
		if target := composeTargetInDir(dir); target.Dir != "" {
			return target
		}
	}
	for _, root := range s.composeSearchRoots() {
		if target := findComposeTargetUnder(root, 5, 400); target.Dir != "" {
			return target
		}
	}
	return systemUpdateComposeTarget{}
}

func (s *SystemUpdateService) composeCandidateDirs() []string {
	var dirs []string
	add := func(values ...string) {
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value != "" {
				dirs = append(dirs, value)
			}
		}
	}
	if wd, err := os.Getwd(); err == nil {
		add(wd)
		add(parentDirs(wd, 4)...)
	}
	if exe, err := os.Executable(); err == nil {
		add(filepath.Dir(exe))
		add(parentDirs(filepath.Dir(exe), 4)...)
	}
	if s != nil && s.cfg != nil {
		add(s.cfg.App.DataDir)
		add(parentDirs(s.cfg.App.DataDir, 4)...)
	}
	return uniqueExistingDirs(dirs)
}

func (s *SystemUpdateService) composeSearchRoots() []string {
	roots := []string{"/data", "/config", "/app", "/opt", "/srv", "/mnt", "/vol1", "/volume1"}
	if runtime.GOOS == "windows" {
		if wd, err := os.Getwd(); err == nil {
			roots = []string{wd}
		}
	}
	return uniqueExistingDirs(roots)
}

func composeTargetInDir(dir string) systemUpdateComposeTarget {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return systemUpdateComposeTarget{}
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return systemUpdateComposeTarget{}
	}
	for _, name := range systemUpdateComposeFiles {
		file := filepath.Join(dir, name)
		if composeFileMatches(file) {
			return systemUpdateComposeTarget{Dir: dir, File: file}
		}
	}
	return systemUpdateComposeTarget{}
}

func findComposeTargetUnder(root string, maxDepth, maxVisited int) systemUpdateComposeTarget {
	root = strings.TrimSpace(root)
	if root == "" || maxDepth < 0 || maxVisited <= 0 {
		return systemUpdateComposeTarget{}
	}
	visited := 0
	var best systemUpdateComposeTarget
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || best.Dir != "" {
			return nil
		}
		if !entry.IsDir() {
			return nil
		}
		visited++
		if visited > maxVisited {
			return filepath.SkipAll
		}
		if depthBeyond(root, path, maxDepth) {
			return filepath.SkipDir
		}
		if target := composeTargetInDir(path); target.Dir != "" {
			best = target
			return filepath.SkipAll
		}
		return nil
	})
	return best
}

func composeFileMatches(file string) bool {
	info, err := os.Stat(file)
	if err != nil || info.IsDir() {
		return false
	}
	raw, err := os.ReadFile(file) // #nosec G304 -- user-controlled compose path is an admin-only local update setting.
	if err != nil {
		return false
	}
	content := strings.ToLower(string(raw))
	return strings.Contains(content, "mediastation-go") || strings.Contains(content, strings.ToLower(DefaultSystemUpdateImage))
}

func preferredComposeCommand() string {
	return "docker compose"
}

func parentDirs(dir string, limit int) []string {
	dir = strings.TrimSpace(dir)
	if dir == "" || limit <= 0 {
		return nil
	}
	parents := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		next := filepath.Dir(dir)
		if next == dir || next == "." || next == "" {
			break
		}
		parents = append(parents, next)
		dir = next
	}
	return parents
}

func uniqueExistingDirs(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		clean := filepath.Clean(value)
		if _, ok := seen[strings.ToLower(clean)]; ok {
			continue
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[strings.ToLower(clean)] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func depthBeyond(root, path string, maxDepth int) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return false
	}
	rel = filepath.ToSlash(rel)
	return strings.Count(rel, "/")+1 > maxDepth
}
