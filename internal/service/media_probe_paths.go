package service

import (
	"context"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const FFprobePathMappingsSettingKey = "ffprobe.path_mappings"

// mapRemoteProbePathWithRoot resolves only explicit URL-prefix mappings. The
// longest complete path prefix wins; traversal and escaping symlinks are refused.
func mapRemoteProbePathWithRoot(rawMappings, rawURL string) (string, string) {
	target, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return "", ""
	}
	bestLength, bestPath, bestRoot := -1, "", ""
	for _, line := range strings.Split(rawMappings, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=>", 2)
		if len(parts) != 2 || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		prefix, err := url.Parse(strings.TrimSpace(parts[0]))
		root := filepath.Clean(strings.TrimSpace(parts[1]))
		if err != nil || prefix.User != nil || prefix.RawQuery != "" || prefix.Fragment != "" || !filepath.IsAbs(root) || !strings.EqualFold(prefix.Scheme, target.Scheme) || !strings.EqualFold(prefix.Host, target.Host) {
			continue
		}
		prefixPath := strings.TrimRight(prefix.Path, "/")
		if target.Path != prefixPath && !strings.HasPrefix(target.Path, prefixPath+"/") {
			continue
		}
		suffix := strings.TrimPrefix(target.Path[len(prefixPath):], "/")
		unsafe := false
		for _, part := range strings.Split(suffix, "/") {
			if part == ".." || strings.ContainsAny(part, "\\:\x00") {
				unsafe = true
			}
		}
		if unsafe || len(prefixPath) <= bestLength {
			continue
		}
		candidate := filepath.Join(root, filepath.FromSlash(suffix))
		if !probePathWithin(root, candidate) {
			continue
		}
		bestLength, bestPath, bestRoot = len(prefixPath), candidate, root
	}
	return bestPath, bestRoot
}

func probePathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func accessibleProbeMapping(rawMappings, rawURL string) (string, string) {
	candidate, root := mapRemoteProbePathWithRoot(rawMappings, rawURL)
	if candidate == "" {
		return "", ""
	}
	realRoot, rootErr := filepath.EvalSymlinks(root)
	realPath, pathErr := filepath.EvalSymlinks(candidate)
	if rootErr != nil || pathErr != nil || !probePathWithin(realRoot, realPath) {
		return "", ""
	}
	info, err := os.Stat(realPath)
	if err != nil || !info.Mode().IsRegular() {
		return "", ""
	}
	return realPath, realRoot
}

func (s *StreamService) mappedProbePath(ctx context.Context, rawURL string) string {
	if s.repo == nil || s.repo.Setting == nil {
		return ""
	}
	raw, err := s.repo.Setting.Get(ctx, FFprobePathMappingsSettingKey)
	if err != nil {
		return ""
	}
	path, _ := accessibleProbeMapping(raw, rawURL)
	return path
}

func probeStableLocal(ctx context.Context, probe interface {
	Probe(context.Context, string) (*ProbeResult, error)
}, path string) (*ProbeResult, error) {
	before, statErr := os.Stat(path)
	result, err := probe.Probe(ctx, path)
	if err != nil {
		return nil, err
	}
	if statErr == nil {
		after, err := os.Stat(path)
		if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return nil, ErrMediaProbeSourceChanged
		}
	}
	return result, nil
}
