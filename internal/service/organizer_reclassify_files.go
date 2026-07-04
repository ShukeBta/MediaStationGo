package service

import (
	"os"
	"path/filepath"
	"strings"
)

func safeToRemoveReclassifiedDuplicate(path, target string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	targetInfo, err := os.Stat(target)
	if err != nil {
		return false
	}
	if os.SameFile(info, targetInfo) {
		return true
	}
	return info.Size() > 0 && info.Size() == targetInfo.Size()
}

func organizeFileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func moveSidecarNFO(oldMedia, newMedia string) error {
	oldNFO := nfoPath(oldMedia)
	newNFO := nfoPath(newMedia)
	if oldNFO == newNFO || !organizeFileExists(oldNFO) || organizeFileExists(newNFO) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(newNFO), 0o755); err != nil { // #nosec G301 -- sidecar media directories must remain readable by NAS/player users.
		return err
	}
	return moveFile(oldNFO, newNFO)
}

func removeMediaAndNFO(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if nfo := nfoPath(path); nfo != "" {
		if err := os.Remove(nfo); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func cleanupEmptyMediaDirs(startDir, stopRoot string) {
	dir := filepath.Clean(strings.TrimSpace(startDir))
	stopRoot = filepath.Clean(strings.TrimSpace(stopRoot))
	for dir != "" && dir != "." {
		if stopRoot != "" && stopRoot != "." && (!pathWithin(dir, stopRoot) || strings.EqualFold(dir, stopRoot)) {
			return
		}
		if err := os.Remove(dir); err != nil {
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return
		}
		dir = parent
	}
}
