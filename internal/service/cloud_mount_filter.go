package service

import (
	"context"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func FilterDisplayCloudLibraries(ctx context.Context, repo *repository.Container, libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	libs = FilterDeprecatedNativeCloudLibraries(libs)
	libs = FilterMergedCloudAutoCategoryLibraries(libs)
	counts := cloudLibraryMediaCounts(ctx, repo, libs)
	collapsed := make([]model.Library, 0, len(libs))
	byKey := make(map[string]int, len(libs))
	for _, lib := range libs {
		key, ok := cloudLibraryDisplayKey(lib)
		if !ok {
			collapsed = append(collapsed, lib)
			continue
		}
		if prevIndex, exists := byKey[key]; exists {
			if betterDisplayCloudLibrary(lib, collapsed[prevIndex], counts) {
				collapsed[prevIndex] = lib
			}
			continue
		}
		byKey[key] = len(collapsed)
		collapsed = append(collapsed, lib)
	}
	collapsed = FilterShadowedCloudLibraries(collapsed)
	return normalizeDisplayLibraries(dedupeDisplayLibrariesByMergeKey(mergeDisplayCloudLibraries(collapsed), counts))
}

func FilterInternalCloudAutoCategoryLibraries(libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if CloudLibraryAutoCategory(lib) {
			continue
		}
		out = append(out, lib)
	}
	return out
}

func FilterMergedCloudAutoCategoryLibraries(libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	nonAutoKeys := make(map[string]struct{}, len(libs))
	for _, lib := range libs {
		if CloudLibraryAutoCategory(lib) {
			continue
		}
		if key, ok := CloudLibraryMergeKey(lib); ok {
			nonAutoKeys[key] = struct{}{}
		}
	}
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		if CloudLibraryAutoCategory(lib) {
			if key, ok := CloudLibraryMergeKey(lib); ok {
				if _, merged := nonAutoKeys[key]; merged {
					continue
				}
			}
		}
		out = append(out, lib)
	}
	return out
}

func FilterScannableCloudLibraries(ctx context.Context, repo *repository.Container, libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	counts := cloudLibraryMediaCounts(ctx, repo, libs)
	collapsed := make([]model.Library, 0, len(libs))
	byKey := make(map[string]int, len(libs))
	for _, lib := range libs {
		if CloudLibraryAutoCategory(lib) {
			continue
		}
		if info, ok := ParseCloudLibraryMount(lib.Path); ok && IsDeprecatedNativeCloudProvider(info.Provider) {
			continue
		}
		key, ok := cloudLibraryDisplayKey(lib)
		if !ok {
			collapsed = append(collapsed, lib)
			continue
		}
		if prevIndex, exists := byKey[key]; exists {
			if betterDisplayCloudLibrary(lib, collapsed[prevIndex], counts) {
				collapsed[prevIndex] = lib
			}
			continue
		}
		byKey[key] = len(collapsed)
		collapsed = append(collapsed, lib)
	}
	return FilterShadowedCloudLibraries(collapsed)
}

func FilterDeprecatedNativeCloudLibraries(libs []model.Library) []model.Library {
	if len(libs) == 0 {
		return libs
	}
	out := make([]model.Library, 0, len(libs))
	for _, lib := range libs {
		info, ok := ParseCloudLibraryMount(lib.Path)
		if ok && IsDeprecatedNativeCloudProvider(info.Provider) {
			continue
		}
		out = append(out, lib)
	}
	return out
}
