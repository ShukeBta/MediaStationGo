package service

import (
	"net/url"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// CloudMountInfo is the canonical identity of a mounted cloud library. ScanDir
// is the provider id/path used for listing. DisplayDir is a hierarchical path
// used to prevent mounting both a parent and its child as separate libraries.
type CloudMountInfo struct {
	Provider   string
	DisplayDir string
	ScanDir    string
	Path       string
}

type CloudMountConflict struct {
	Library            model.Library `json:"library"`
	Exact              bool          `json:"exact"`
	Nested             bool          `json:"nested"`
	ExistingIsAncestor bool          `json:"existing_is_ancestor"`
}

func BuildCloudLibraryPath(provider, scanDir, displayDir string) string {
	provider = strings.TrimSpace(provider)
	scanDir = normalizeCloudMountDir(provider, scanDir)
	displayDir = normalizeCloudMountDir(provider, firstNonEmpty(displayDir, scanDir))
	if provider == "" {
		return ""
	}
	base := "cloud://" + provider
	if displayDir == "" {
		if scanDir != "" {
			return base + "?dir=" + url.QueryEscape(scanDir)
		}
		return base
	}
	path := base + "/" + url.PathEscape(displayDir)
	if scanDir != "" && scanDir != displayDir {
		path += "?dir=" + url.QueryEscape(scanDir)
	}
	return path
}

func ParseCloudLibraryMount(raw string) (CloudMountInfo, bool) {
	raw = strings.TrimSpace(raw)
	if !strings.HasPrefix(strings.ToLower(raw), "cloud://") {
		return CloudMountInfo{}, false
	}
	u, err := url.Parse(raw)
	if err != nil || strings.ToLower(u.Scheme) != "cloud" {
		return CloudMountInfo{}, false
	}
	provider := strings.TrimSpace(u.Host)
	if provider == "" {
		return CloudMountInfo{}, false
	}
	displayDir := strings.Trim(strings.TrimSpace(u.Path), "/")
	if decoded, err := url.PathUnescape(displayDir); err == nil {
		displayDir = decoded
	}
	scanDir := displayDir
	if qDir := strings.TrimSpace(u.Query().Get("dir")); qDir != "" {
		if decoded, err := url.QueryUnescape(qDir); err == nil {
			qDir = decoded
		}
		scanDir = qDir
	}
	displayDir = normalizeCloudMountDir(provider, displayDir)
	scanDir = normalizeCloudMountDir(provider, scanDir)
	return CloudMountInfo{
		Provider:   provider,
		DisplayDir: displayDir,
		ScanDir:    scanDir,
		Path:       raw,
	}, true
}

func FindCloudMountConflict(libs []model.Library, provider, scanDir, displayDir string) *CloudMountConflict {
	candidate := CloudMountInfo{
		Provider:   strings.TrimSpace(provider),
		DisplayDir: normalizeCloudMountDir(provider, firstNonEmpty(displayDir, scanDir)),
		ScanDir:    normalizeCloudMountDir(provider, scanDir),
	}
	for _, lib := range libs {
		if CloudLibraryAutoCategory(lib) {
			continue
		}
		existing, ok := ParseCloudLibraryMount(lib.Path)
		if !ok || existing.Provider != candidate.Provider {
			continue
		}
		if existing.DisplayDir == candidate.DisplayDir {
			return &CloudMountConflict{Library: lib, Exact: true}
		}
		if existing.ScanDir != "" && candidate.ScanDir != "" && existing.ScanDir == candidate.ScanDir {
			return &CloudMountConflict{Library: lib, Exact: true}
		}
		if cloudMountAncestor(candidate.DisplayDir, existing.DisplayDir) {
			return &CloudMountConflict{Library: lib, Nested: true}
		}
	}
	return nil
}

func CloudLibraryShadowed(libs []model.Library, lib model.Library) *CloudMountConflict {
	current, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return nil
	}
	for _, existing := range libs {
		if existing.ID == lib.ID || !existing.Enabled {
			continue
		}
		if CloudLibraryAutoCategory(existing) {
			continue
		}
		info, ok := ParseCloudLibraryMount(existing.Path)
		if !ok || info.Provider != current.Provider {
			continue
		}
		if info.DisplayDir == current.DisplayDir && existing.CreatedAt.Before(lib.CreatedAt) {
			return &CloudMountConflict{Library: existing, Exact: true}
		}
		if cloudMountAncestor(current.DisplayDir, info.DisplayDir) {
			return &CloudMountConflict{Library: existing, Nested: true}
		}
	}
	return nil
}

func FilterShadowedCloudLibraries(libs []model.Library) []model.Library {
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if CloudLibraryShadowed(libs, lib) == nil {
			out = append(out, lib)
		}
	}
	return out
}
