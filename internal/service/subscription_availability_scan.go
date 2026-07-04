package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

func sameOrChildPath(pathValue, root string) bool {
	pathValue = filepath.Clean(strings.TrimSpace(pathValue))
	root = filepath.Clean(strings.TrimSpace(root))
	if pathValue == "" || root == "" || pathValue == "." || root == "." {
		return false
	}
	if strings.EqualFold(pathValue, root) {
		return true
	}
	rel, err := filepath.Rel(root, pathValue)
	if err != nil {
		return false
	}
	return rel != "." && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)
}

func scanDownloadPath(ctx context.Context, root, query string, visit func(path string, season, episode int) bool) error {
	return scanDownloadPathAny(ctx, root, []string{query}, visit)
}

func scanDownloadPathAny(ctx context.Context, root string, queries []string, visit func(path string, season, episode int) bool) error {
	if strings.TrimSpace(root) == "" {
		return nil
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return nil
	}
	if len(queries) == 0 {
		return nil
	}
	visited := 0
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			if path != root && strings.HasPrefix(filepath.Base(path), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !isDownloadMediaPath(path) {
			return nil
		}
		visited++
		if visited > 10000 {
			return filepath.SkipAll
		}
		if !availabilityTitleMatchesAny(path, queries) {
			return nil
		}
		season, episode := ParseEpisode(path)
		if !visit(path, season, episode) {
			return filepath.SkipAll
		}
		return nil
	})
}

func isDownloadMediaPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".!qb", ".part", ".aria2", ".crdownload":
		path = strings.TrimSuffix(path, filepath.Ext(path))
		ext = strings.ToLower(filepath.Ext(path))
	}
	_, ok := videoExtensions[ext]
	return ok
}

func normalizeAvailabilityComparable(value string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}
