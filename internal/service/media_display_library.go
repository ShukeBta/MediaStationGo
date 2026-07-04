package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func (s *MediaService) attachLibraryMetadata(ctx context.Context, items []model.Media) {
	if s == nil || s.repo == nil || s.repo.Library == nil || len(items) == 0 {
		return
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return
	}
	byID := make(map[string]model.Library, len(libs))
	for i := range libs {
		libs[i] = normalizeLocalLibraryPathForDisplay(libs[i])
		lib := libs[i]
		byID[lib.ID] = lib
	}
	resolver := newMediaDisplayLibraryResolver(ctx, s.repo, libs)
	for i := range items {
		var own model.Library
		var hasOwn bool
		if lib, ok := byID[items[i].LibraryID]; ok {
			own = lib
			hasOwn = true
			items[i].LibraryName = lib.Name
			items[i].LibraryPath = lib.Path
		}
		if lib, ok := resolver.DisplayLibraryForMedia(items[i]); ok {
			items[i].DisplayLibraryID = lib.ID
			items[i].DisplayLibraryName = lib.Name
			items[i].DisplayLibraryPath = lib.Path
			if hasOwn && CloudLibraryAutoCategory(own) {
				items[i].LibraryName = lib.Name
				items[i].LibraryPath = lib.Path
			}
		}
	}
}

type mediaDisplayLibraryResolver struct {
	byID              map[string]model.Library
	displayByID       map[string]model.Library
	displayByMergeKey map[string]model.Library
	displayLibraries  []model.Library
}

func newMediaDisplayLibraryResolver(ctx context.Context, repo *repository.Container, libs []model.Library) mediaDisplayLibraryResolver {
	displayLibraries := FilterDisplayCloudLibraries(ctx, repo, append([]model.Library(nil), libs...))
	resolver := mediaDisplayLibraryResolver{
		byID:              make(map[string]model.Library, len(libs)),
		displayByID:       make(map[string]model.Library, len(displayLibraries)),
		displayByMergeKey: make(map[string]model.Library, len(displayLibraries)),
		displayLibraries:  displayLibraries,
	}
	for _, lib := range libs {
		normalized := normalizeLocalLibraryPathForDisplay(lib)
		resolver.byID[normalized.ID] = normalized
	}
	for _, lib := range displayLibraries {
		resolver.displayByID[lib.ID] = lib
		if key, ok := CloudLibraryMergeKey(lib); ok {
			if _, exists := resolver.displayByMergeKey[key]; !exists {
				resolver.displayByMergeKey[key] = lib
			}
		}
	}
	return resolver
}

func (r mediaDisplayLibraryResolver) DisplayLibraryForMedia(media model.Media) (model.Library, bool) {
	// Issue #61: an auto-category assignment is authoritative. The media was explicitly
	// categorized into this library even though its physical (cloud) path may still live
	// under the source scan directory (e.g. cloud://cloud115/云下载/...). Resolving by path
	// here would wrongly redirect the media back to the source cloud library, so resolve it
	// from the owning library instead.
	if own, ok := r.byID[media.LibraryID]; ok && CloudLibraryAutoCategory(own) {
		return r.autoCategoryDisplayLibrary(own), true
	}
	if lib, ok := r.bestPathDisplayLibrary(media); ok {
		return lib, true
	}
	if lib, ok := r.displayByID[media.LibraryID]; ok {
		return lib, true
	}
	if own, hasOwn := r.byID[media.LibraryID]; hasOwn {
		if key, ok := CloudLibraryMergeKey(own); ok {
			if lib, exists := r.displayByMergeKey[key]; exists {
				return lib, true
			}
		}
		return own, true
	}
	return model.Library{}, false
}

// autoCategoryDisplayLibrary resolves the visible library that should represent an
// auto-category library: the library itself when it is displayed standalone, otherwise
// the sibling it was merged into, otherwise the root cloud library it was split from.
func (r mediaDisplayLibraryResolver) autoCategoryDisplayLibrary(own model.Library) model.Library {
	if lib, ok := r.displayByID[own.ID]; ok {
		return lib
	}
	if key, ok := CloudLibraryMergeKey(own); ok {
		if lib, exists := r.displayByMergeKey[key]; exists {
			return lib
		}
	}
	if lib, ok := r.rootCloudDisplayLibraryForAutoCategory(own); ok {
		return lib
	}
	return own
}

func (r mediaDisplayLibraryResolver) rootCloudDisplayLibraryForAutoCategory(auto model.Library) (model.Library, bool) {
	info, ok := ParseCloudLibraryMount(auto.Path)
	if !ok {
		return model.Library{}, false
	}
	for _, lib := range r.displayLibraries {
		if !lib.Enabled {
			continue
		}
		candidate, ok := ParseCloudLibraryMount(lib.Path)
		if ok && candidate.Provider == info.Provider && cloudRootMountNeedsAutoCategory(candidate) {
			return lib, true
		}
	}
	return model.Library{}, false
}

func (r mediaDisplayLibraryResolver) bestPathDisplayLibrary(media model.Media) (model.Library, bool) {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(media.Path)), "cloud://") {
		mediaInfo, ok := ParseCloudLibraryMount(media.Path)
		if !ok {
			return model.Library{}, false
		}
		var best model.Library
		bestDepth := 0
		for _, lib := range r.displayLibraries {
			info, ok := ParseCloudLibraryMount(lib.Path)
			if !ok || info.Provider != mediaInfo.Provider || !lib.Enabled {
				continue
			}
			dir := strings.Trim(firstNonEmpty(info.DisplayDir, info.ScanDir), "/")
			if dir == "" {
				continue
			}
			mediaDir := strings.Trim(firstNonEmpty(mediaInfo.DisplayDir, mediaInfo.ScanDir), "/")
			if mediaDir != dir && !cloudMountAncestor(dir, mediaDir) {
				continue
			}
			depth := len(strings.Split(dir, "/"))
			if depth > bestDepth {
				best = lib
				bestDepth = depth
			}
		}
		if bestDepth > 0 {
			return best, true
		}
		return model.Library{}, false
	}

	mediaPath := cleanPathForVolumeMapping(media.Path)
	if isRelativeVolumeMarkerPath(media.Path) {
		mediaPath = cleanPathForVolumeMapping(resolveMappedDestinationPath(media.Path))
	}
	var best model.Library
	bestLen := 0
	for _, lib := range r.displayLibraries {
		if _, ok := ParseCloudLibraryMount(lib.Path); ok || !lib.Enabled {
			continue
		}
		libPath := cleanPathForVolumeMapping(resolveMappedDestinationPath(lib.Path))
		if libPath == "" || libPath == "." {
			continue
		}
		if mediaPath != libPath && !strings.HasPrefix(mediaPath, strings.TrimRight(libPath, "/")+"/") {
			continue
		}
		if len(libPath) > bestLen {
			best = lib
			bestLen = len(libPath)
		}
	}
	if bestLen > 0 {
		return best, true
	}
	return model.Library{}, false
}

func normalizeLocalLibraryPathForDisplay(lib model.Library) model.Library {
	if _, ok := ParseCloudLibraryMount(lib.Path); ok {
		return lib
	}
	lib.Path = resolveMappedDestinationPath(lib.Path)
	for i := range lib.Roots {
		if _, ok := ParseCloudLibraryMount(lib.Roots[i].Path); ok {
			continue
		}
		lib.Roots[i].Path = resolveMappedDestinationPath(lib.Roots[i].Path)
	}
	return lib
}
