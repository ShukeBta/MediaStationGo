package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type activeDownloadGuard struct {
	paths []string
}

func (o *OrganizerService) newActiveDownloadGuard(ctx context.Context) activeDownloadGuard {
	if o == nil || o.activeDownloadPaths == nil {
		return activeDownloadGuard{}
	}
	return activeDownloadGuard{paths: cleanUniqueExistingPaths(o.activeDownloadPaths(ctx))}
}

func (g activeDownloadGuard) contains(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." {
		return false
	}
	for _, root := range g.paths {
		if pathWithin(path, root) {
			return true
		}
	}
	return false
}

func activeDownloadPathCandidates(torrents []QBitTorrent, mappings map[string]string) []string {
	if len(torrents) == 0 {
		return nil
	}
	var out []string
	for _, torrent := range torrents {
		if qbitTorrentCompleted(torrent) {
			continue
		}
		for _, raw := range []string{
			torrent.ContentPath,
			filepath.Join(strings.TrimSpace(torrent.SavePath), strings.TrimSpace(torrent.Name)),
		} {
			out = appendDownloadPathCandidates(out, raw, mappings)
		}
	}
	return cleanUniqueExistingPaths(out)
}

func appendDownloadPathCandidates(out []string, raw string, mappings map[string]string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "." {
		return out
	}
	if translated := translateClientPath(raw, mappings); translated != "" {
		out = append(out, translated)
	}
	for _, candidate := range mappedPathCandidates(raw) {
		if _, err := os.Stat(candidate); err == nil {
			out = append(out, candidate)
		}
	}
	return out
}

func cleanUniqueExistingPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = filepath.Clean(strings.TrimSpace(path))
		if path == "" || path == "." {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		duplicate := false
		for _, existing := range out {
			if sameLibraryPath(existing, path) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out = append(out, path)
		}
	}
	return out
}
