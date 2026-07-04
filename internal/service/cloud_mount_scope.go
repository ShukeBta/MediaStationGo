package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func MergedLibraryIDsForLibrary(ctx context.Context, repo *repository.Container, libraryID string) ([]string, error) {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" || repo == nil || repo.Library == nil {
		return []string{libraryID}, nil
	}
	lib, err := repo.Library.FindByID(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	if lib == nil {
		return []string{libraryID}, nil
	}
	libs, err := repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	return MergedLibraryIDs(libs, *lib), nil
}

func MergedLibraryIDs(libs []model.Library, target model.Library) []string {
	ids := appendUniqueLibraryIDs(nil, target.ID)
	if rootAutoIDs := cloudRootAutoCategoryLibraryIDs(libs, target); len(rootAutoIDs) > 0 {
		ids = appendUniqueLibraryIDs(ids, rootAutoIDs...)
	}
	targetKey, hasTargetKey := CloudLibraryMergeKey(target)
	if !hasTargetKey {
		return ids
	}
	_, targetIsCloud := ParseCloudLibraryMount(target.Path)
	for _, candidate := range libs {
		if candidate.ID == target.ID || strings.TrimSpace(candidate.ID) == "" || !candidate.Enabled {
			continue
		}
		key, ok := CloudLibraryMergeKey(candidate)
		if ok && key == targetKey {
			_, candidateIsCloud := ParseCloudLibraryMount(candidate.Path)
			if !targetIsCloud && !candidateIsCloud {
				continue
			}
			ids = appendUniqueLibraryIDs(ids, candidate.ID)
		}
	}
	return ids
}

func cloudRootAutoCategoryLibraryIDs(libs []model.Library, lib model.Library) []string {
	mount, ok := ParseCloudLibraryMount(lib.Path)
	if !ok || !cloudRootMountNeedsAutoCategory(mount) {
		return nil
	}
	ids := make([]string, 0)
	for _, candidate := range libs {
		if candidate.ID == lib.ID || !candidate.Enabled || !CloudLibraryAutoCategory(candidate) {
			continue
		}
		info, ok := ParseCloudLibraryMount(candidate.Path)
		if ok && info.Provider == mount.Provider {
			ids = appendUniqueLibraryIDs(ids, candidate.ID)
		}
	}
	return ids
}

func ExpandMediaVisibilityForMergedCloudLibraries(ctx context.Context, repo *repository.Container, visibility MediaVisibility) MediaVisibility {
	if repo == nil || repo.Library == nil {
		return visibility
	}
	libs, err := repo.Library.List(ctx)
	if err != nil {
		return visibility
	}
	if len(visibility.AllowedLibraryIDs) > 0 {
		visibility.AllowedLibraryIDs = expandMergedLibraryIDsFromLibraries(libs, visibility.AllowedLibraryIDs)
	}
	if len(visibility.HiddenLibraryIDs) > 0 {
		visibility.HiddenLibraryIDs = expandMergedLibraryIDsFromLibraries(libs, visibility.HiddenLibraryIDs)
	}
	visibility.HiddenLibraryIDs = appendUniqueLibraryIDs(visibility.HiddenLibraryIDs, DeprecatedNativeCloudLibraryIDs(libs)...)
	return visibility
}

func expandMergedLibraryIDs(ctx context.Context, repo *repository.Container, ids []string) []string {
	if len(ids) == 0 || repo == nil || repo.Library == nil {
		return ids
	}
	libs, err := repo.Library.List(ctx)
	if err != nil {
		return ids
	}
	return expandMergedLibraryIDsFromLibraries(libs, ids)
}

func expandMergedLibraryIDsFromLibraries(libs []model.Library, ids []string) []string {
	byID := make(map[string]model.Library, len(libs))
	for _, lib := range libs {
		byID[lib.ID] = lib
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if lib, ok := byID[id]; ok {
			out = appendUniqueLibraryIDs(out, MergedLibraryIDs(libs, lib)...)
			continue
		}
		out = appendUniqueLibraryIDs(out, id)
	}
	return out
}

func DeprecatedNativeCloudLibraryIDs(libs []model.Library) []string {
	ids := make([]string, 0)
	for _, lib := range libs {
		info, ok := ParseCloudLibraryMount(lib.Path)
		if ok && IsDeprecatedNativeCloudProvider(info.Provider) {
			ids = appendUniqueLibraryIDs(ids, lib.ID)
		}
	}
	return ids
}

func appendUniqueLibraryIDs(ids []string, values ...string) []string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		exists := false
		for _, id := range ids {
			if id == value {
				exists = true
				break
			}
		}
		if !exists {
			ids = append(ids, value)
		}
	}
	return ids
}
