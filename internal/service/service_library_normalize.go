package service

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (c *Container) NormalizeLocalLibraryPaths(ctx context.Context) error {
	if c == nil || c.Repo == nil || c.Repo.Library == nil || c.Repo.DB == nil {
		return nil
	}
	libs, err := c.Repo.Library.List(ctx)
	if err != nil {
		return err
	}
	for _, lib := range libs {
		if _, ok := ParseCloudLibraryMount(lib.Path); ok {
			continue
		}
		if len(lib.Roots) == 0 {
			normalized := normalizePersistedLocalLibraryPath(lib.Path)
			if sameLibraryPath(normalized, lib.Path) {
				continue
			}
			if err := c.Repo.DB.WithContext(ctx).
				Model(&model.Library{}).
				Where("id = ?", lib.ID).
				Update("path", normalized).Error; err != nil {
				return err
			}
			continue
		}
		primaryPath := ""
		for i, root := range lib.Roots {
			if _, ok := ParseCloudLibraryMount(root.Path); ok {
				if i == 0 {
					primaryPath = root.Path
				}
				continue
			}
			normalized := normalizePersistedLocalLibraryPath(root.Path)
			if i == 0 {
				primaryPath = normalized
			}
			if sameLibraryPath(normalized, root.Path) || strings.TrimSpace(root.ID) == "" {
				continue
			}
			if err := c.Repo.DB.WithContext(ctx).
				Model(&model.LibraryRoot{}).
				Where("id = ?", root.ID).
				Update("path", normalized).Error; err != nil {
				return err
			}
		}
		if strings.TrimSpace(primaryPath) != "" && !sameLibraryPath(primaryPath, lib.Path) {
			if err := c.Repo.DB.WithContext(ctx).
				Model(&model.Library{}).
				Where("id = ?", lib.ID).
				Update("path", primaryPath).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizePersistedLocalLibraryPath(pathValue string) string {
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		return ""
	}
	if !isRelativeVolumeMarkerPath(pathValue) {
		return filepath.Clean(pathValue)
	}
	return resolveMappedDestinationPath(pathValue)
}

func (c *Container) NormalizeCloudLibraryTypes(ctx context.Context) error {
	if c == nil || c.Repo == nil || c.Repo.Library == nil || c.Repo.DB == nil {
		return nil
	}
	libs, err := c.Repo.Library.List(ctx)
	if err != nil {
		return err
	}
	for _, lib := range libs {
		info, ok := ParseCloudLibraryMount(lib.Path)
		if !ok {
			continue
		}
		want := InferCloudMountMediaType(info.DisplayDir, lib.Name)
		if want == "" || want == lib.Type {
			continue
		}
		if err := c.Repo.DB.WithContext(ctx).
			Model(&model.Library{}).
			Where("id = ?", lib.ID).
			Update("type", want).Error; err != nil {
			return err
		}
	}
	return nil
}
