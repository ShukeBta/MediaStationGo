package service

import (
	"os"
	"path/filepath"
	"strings"
)

func cleanPathForVolumeMapping(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = strings.ReplaceAll(path, "\\", "/")
	path = trimEmbeddedWindowsDrive(path)
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
}

func pathAfterWindowsDrivePrefix(path string) string {
	if len(path) >= 3 && path[1] == ':' && path[2] == '/' && isASCIIAlpha(path[0]) {
		return path[2:]
	}
	return path
}

func trimEmbeddedWindowsDrive(path string) string {
	for i := 0; i+2 < len(path); i++ {
		if !isASCIIAlpha(path[i]) || path[i+1] != ':' || path[i+2] != '/' {
			continue
		}
		if i == 0 || path[i-1] == '/' {
			return path[i:]
		}
	}
	return path
}

func isASCIIAlpha(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func sameLibraryPath(a, b string) bool {
	return filepath.Clean(a) == filepath.Clean(b)
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
