package database

import (
	"path/filepath"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func ensureLibraryRootsCompatibility(db *gorm.DB) error {
	var libraries []model.Library
	if err := db.Find(&libraries).Error; err != nil {
		return err
	}
	for _, lib := range libraries {
		if strings.TrimSpace(lib.Path) == "" {
			continue
		}
		var count int64
		if err := db.Model(&model.LibraryRoot{}).Where("library_id = ?", lib.ID).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		root := model.LibraryRoot{
			LibraryID: lib.ID,
			Name:      firstLibraryRootLabel(lib.Path),
			Path:      lib.Path,
			Enabled:   lib.Enabled,
			SortOrder: 0,
		}
		if err := db.Create(&root).Error; err != nil {
			return err
		}
		if err := backfillLibraryRootMedia(db, lib, root); err != nil {
			return err
		}
	}
	return nil
}

func backfillLibraryRootMedia(db *gorm.DB, lib model.Library, root model.LibraryRoot) error {
	var rows []model.Media
	if err := db.Unscoped().
		Model(&model.Media{}).
		Select("id", "path").
		Where("library_id = ? AND (library_root_id = '' OR library_root_id IS NULL)", lib.ID).
		Find(&rows).Error; err != nil {
		return err
	}
	rootPath := strings.TrimSpace(root.Path)
	for _, row := range rows {
		rel, ok := relativePathWithinRoot(row.Path, rootPath)
		if !ok {
			continue
		}
		if err := db.Unscoped().Model(&model.Media{}).Where("id = ?", row.ID).Updates(map[string]any{
			"library_root_id": root.ID,
			"relative_path":   rel,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func relativePathWithinRoot(pathValue, root string) (string, bool) {
	pathValue = strings.TrimSpace(pathValue)
	root = strings.TrimSpace(root)
	if pathValue == "" || root == "" {
		return "", false
	}
	if strings.HasPrefix(strings.ToLower(root), "cloud://") || strings.HasPrefix(strings.ToLower(pathValue), "cloud://") {
		prefix := strings.TrimRight(root, "/") + "/"
		if strings.EqualFold(pathValue, root) {
			return "", true
		}
		if strings.HasPrefix(strings.ToLower(pathValue), strings.ToLower(prefix)) {
			return strings.TrimPrefix(pathValue, prefix), true
		}
		return "", false
	}
	cleanPath := filepath.Clean(pathValue)
	cleanRoot := filepath.Clean(root)
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return "", false
	}
	return rel, true
}

func firstLibraryRootLabel(pathValue string) string {
	pathValue = strings.TrimSpace(pathValue)
	if pathValue == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(pathValue), "cloud://") {
		parts := strings.Split(strings.Trim(pathValue, "/"), "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
	}
	base := filepath.Base(filepath.Clean(pathValue))
	if base == "." || base == string(filepath.Separator) {
		return pathValue
	}
	return base
}
