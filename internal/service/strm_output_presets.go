package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type STRMOutputPreset struct {
	Label string `json:"label"`
	Path  string `json:"path"`
	Kind  string `json:"kind"`
}

func STRMOutputPresets(ctx context.Context, repo *repository.Container) ([]STRMOutputPreset, error) {
	presets := defaultSTRMOutputPresets()
	seen := make(map[string]struct{}, len(presets))
	for _, preset := range presets {
		seen[strmOutputPresetKey(preset.Path)] = struct{}{}
	}
	if repo == nil || repo.Library == nil {
		return presets, nil
	}
	libraries, err := repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	for i := range libraries {
		lib := libraries[i]
		if !lib.Enabled {
			continue
		}
		roots, err := repo.Library.ListRoots(ctx, lib.ID)
		if err != nil {
			return nil, err
		}
		if len(roots) == 0 {
			roots = []model.LibraryRoot{{Path: lib.Path, Enabled: lib.Enabled}}
		}
		for j := range roots {
			root := roots[j]
			if !root.Enabled || strings.TrimSpace(root.Path) == "" {
				continue
			}
			if _, ok := ParseCloudLibraryMount(root.Path); ok {
				continue
			}
			pathValue := filepath.Clean(resolveMappedDestinationPath(root.Path))
			if pathValue == "" || pathValue == "." {
				continue
			}
			key := strmOutputPresetKey(pathValue)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			presets = append(presets, STRMOutputPreset{
				Label: lib.Name,
				Path:  pathValue,
				Kind:  "library",
			})
		}
	}
	return presets, nil
}

func defaultSTRMOutputPresets() []STRMOutputPreset {
	return []STRMOutputPreset{
		{Label: "STRM 根目录", Path: filepath.Clean("data/strm"), Kind: "default"},
		{Label: "目录树 STRM", Path: filepath.Clean("data/strm/tree"), Kind: "default"},
	}
}

func strmOutputPresetKey(pathValue string) string {
	return strings.ToLower(filepath.Clean(strings.TrimSpace(pathValue)))
}
