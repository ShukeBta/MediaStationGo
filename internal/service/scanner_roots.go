package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) localLibraryScanRoots(ctx context.Context, lib *model.Library) ([]model.LibraryRoot, error) {
	if lib == nil {
		return nil, errors.New("library not found")
	}
	roots := append([]model.LibraryRoot(nil), lib.Roots...)
	if len(roots) == 0 && s != nil && s.repo != nil && s.repo.Library != nil {
		var err error
		roots, err = s.repo.Library.ListRoots(ctx, lib.ID)
		if err != nil {
			return nil, err
		}
	}
	if len(roots) == 0 && strings.TrimSpace(lib.Path) != "" {
		roots = []model.LibraryRoot{{
			LibraryID: lib.ID,
			Path:      lib.Path,
			Enabled:   lib.Enabled,
		}}
	}
	out := roots[:0]
	for _, root := range roots {
		if !root.Enabled || strings.TrimSpace(root.Path) == "" {
			continue
		}
		if _, ok := ParseCloudLibraryMount(root.Path); ok {
			continue
		}
		out = append(out, root)
	}
	return out, nil
}

func (s *ScannerService) localLibraryRootForPath(ctx context.Context, lib *model.Library, pathValue string) (*model.LibraryRoot, error) {
	roots, err := s.localLibraryScanRoots(ctx, lib)
	if err != nil {
		return nil, err
	}
	cleanPath := filepath.Clean(strings.TrimSpace(pathValue))
	for i := range roots {
		rootPath := filepath.Clean(strings.TrimSpace(roots[i].Path))
		if sameLibraryPath(cleanPath, rootPath) || pathWithin(cleanPath, rootPath) {
			return &roots[i], nil
		}
	}
	if len(roots) > 0 {
		return &roots[0], nil
	}
	return nil, errors.New("library has no enabled paths")
}

func (s *ScannerService) resolveLocalLibraryRootPath(ctx context.Context, lib *model.Library, root *model.LibraryRoot) error {
	if root == nil || strings.TrimSpace(root.Path) == "" {
		return nil
	}
	resolved, err := resolveAccessibleLibraryPath(root.Path)
	if err != nil {
		return err
	}
	if sameLibraryPath(resolved, root.Path) {
		root.Path = filepath.Clean(root.Path)
		return nil
	}
	if s.repo != nil && s.repo.DB != nil && strings.TrimSpace(root.ID) != "" {
		if updateErr := s.repo.DB.WithContext(ctx).Model(&model.LibraryRoot{}).Where("id = ?", root.ID).Update("path", resolved).Error; updateErr != nil && s.log != nil {
			s.log.Warn("update mapped library root path failed",
				zap.String("library_id", lib.ID),
				zap.String("root_id", root.ID),
				zap.String("from", root.Path),
				zap.String("to", resolved),
				zap.Error(updateErr))
		}
	}
	if lib != nil {
		if strings.TrimSpace(root.ID) == "" {
			_ = s.repo.DB.WithContext(ctx).Model(&model.Library{}).Where("id = ?", lib.ID).Update("path", resolved).Error
			lib.Path = resolved
		} else if roots, err := s.repo.Library.ListRoots(ctx, lib.ID); err == nil && len(roots) > 0 && roots[0].ID == root.ID {
			_ = s.repo.DB.WithContext(ctx).Model(&model.Library{}).Where("id = ?", lib.ID).Update("path", resolved).Error
			lib.Path = resolved
		}
	}
	if s.log != nil {
		s.log.Info("mapped library root path for scan",
			zap.String("library_id", lib.ID),
			zap.String("root_id", root.ID),
			zap.String("from", root.Path),
			zap.String("to", resolved))
	}
	root.Path = resolved
	return nil
}

func libraryRootID(root *model.LibraryRoot) string {
	if root == nil {
		return ""
	}
	return strings.TrimSpace(root.ID)
}

func localRelativePath(pathValue string, root *model.LibraryRoot) string {
	if root == nil || strings.TrimSpace(root.Path) == "" || strings.TrimSpace(pathValue) == "" {
		return ""
	}
	rel, err := filepath.Rel(filepath.Clean(root.Path), filepath.Clean(pathValue))
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
		return ""
	}
	return rel
}
