package service

import (
	"context"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const cloudAutoCategoryQueryKey = "auto_category"

func BuildCloudAutoCategoryLibraryPath(provider, displayDir string) string {
	return BuildCloudAutoCategoryLibraryPathWithScanDir(provider, "", displayDir)
}

func BuildCloudAutoCategoryLibraryPathWithScanDir(provider, scanDir, displayDir string) string {
	base := BuildCloudLibraryPath(provider, scanDir, displayDir)
	if base == "" || strings.TrimSpace(displayDir) == "" {
		return ""
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + cloudAutoCategoryQueryKey + "=1"
}

func CloudLibraryAutoCategory(lib model.Library) bool {
	u, err := url.Parse(strings.TrimSpace(lib.Path))
	if err != nil || strings.ToLower(u.Scheme) != "cloud" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(u.Query().Get(cloudAutoCategoryQueryKey))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func cloudRootMountNeedsAutoCategory(mount CloudMountInfo) bool {
	return strings.TrimSpace(mount.DisplayDir) == "" && strings.TrimSpace(mount.ScanDir) == ""
}

func cloudAutoCategoryDisplayDirForMediaPath(path string) string {
	displayDir, _ := cloudAutoCategoryDirsForMediaPath(path)
	return displayDir
}

func cloudAutoCategoryDirsForMediaPath(path string) (string, string) {
	info, ok := ParseCloudLibraryMount(path)
	if !ok {
		return "", ""
	}
	parts := strmSlashParts(info.DisplayDir)
	if len(parts) <= 1 {
		return "", ""
	}
	parts = parts[:len(parts)-1]
	categoryParts, scanParts := cloudAutoCategoryParts(parts)
	if len(categoryParts) == 0 {
		return "", ""
	}
	return strings.Join(categoryParts, "/"), strings.Join(scanParts, "/")
}

func cloudAutoCategoryParts(parts []string) ([]string, []string) {
	for i, part := range parts {
		root := strmCanonicalRoot(part)
		if root != "" {
			if i+1 >= len(parts) {
				return nil, nil
			}
			category := strings.TrimSpace(parts[i+1])
			if cloudAutoCategoryRootMatches(root, category) {
				return []string{root, strmCanonicalCategory(category)}, append([]string(nil), parts[:i+2]...)
			}
			return nil, nil
		}
		if root := strmCategoryRoot(part); root != "" {
			return []string{root, strmCanonicalCategory(part)}, append([]string(nil), parts[:i+1]...)
		}
	}
	return nil, nil
}

func cloudAutoCategoryRootMatches(root, category string) bool {
	category = strings.TrimSpace(category)
	if category == "" {
		return false
	}
	if strmCategoryRoot(category) == root {
		return true
	}
	if root == "电影" {
		return containsAnyText(strings.ToLower(category), "纪录片", "纪录", "documentary")
	}
	return false
}

type cloudAutoCategoryTarget struct {
	Library *model.Library
	RootID  string
}

func (s *ScannerService) ensureCloudAutoCategoryTarget(ctx context.Context, rootLib *model.Library, provider, displayDir, scanDir string) (cloudAutoCategoryTarget, error) {
	displayDir = normalizeCloudMountDir(provider, displayDir)
	scanDir = normalizeCloudMountDir(provider, firstNonEmpty(scanDir, displayDir))
	if s == nil || s.repo == nil || s.repo.DB == nil || rootLib == nil || provider == "" || displayDir == "" {
		return cloudAutoCategoryTarget{Library: rootLib}, nil
	}
	path := BuildCloudAutoCategoryLibraryPathWithScanDir(provider, scanDir, displayDir)
	if path == "" {
		return cloudAutoCategoryTarget{Library: rootLib}, nil
	}
	name := cloudMountDirBase(displayDir)
	if name == "" {
		name = displayDir
	}
	kind := InferCloudMountMediaType(displayDir, name)
	target, existingAuto := s.findCloudAutoCategoryTarget(ctx, rootLib.ID, provider, displayDir, name, kind)
	if target != nil {
		root, err := s.ensureCloudLibraryRoot(ctx, target.ID, name, path)
		if err != nil {
			return cloudAutoCategoryTarget{}, err
		}
		if existingAuto != nil && existingAuto.ID != target.ID {
			s.migrateCloudAutoCategoryLibrary(ctx, existingAuto, target, root)
		}
		return cloudAutoCategoryTarget{Library: target, RootID: libraryRootID(root)}, nil
	}
	if existingAuto != nil {
		root, err := s.ensureCloudLibraryRoot(ctx, existingAuto.ID, name, path)
		if err != nil {
			return cloudAutoCategoryTarget{}, err
		}
		return cloudAutoCategoryTarget{Library: existingAuto, RootID: libraryRootID(root)}, nil
	}
	lib := &model.Library{
		Name:    name,
		Path:    path,
		Type:    kind,
		Enabled: true,
	}
	root := model.LibraryRoot{Name: name, Path: path, Enabled: true}
	if err := s.repo.Library.CreateWithRoots(ctx, lib, []model.LibraryRoot{root}); err != nil {
		_, existing := s.findCloudAutoCategoryTarget(ctx, rootLib.ID, provider, displayDir, name, kind)
		if existing != nil {
			ensuredRoot, rootErr := s.ensureCloudLibraryRoot(ctx, existing.ID, name, path)
			if rootErr != nil {
				return cloudAutoCategoryTarget{}, rootErr
			}
			return cloudAutoCategoryTarget{Library: existing, RootID: libraryRootID(ensuredRoot)}, nil
		}
		return cloudAutoCategoryTarget{}, err
	}
	if s.log != nil {
		s.log.Info("created cloud auto category library",
			zap.String("root_library_id", rootLib.ID),
			zap.String("library_id", lib.ID),
			zap.String("provider", provider),
			zap.String("display_dir", displayDir))
	}
	if len(lib.Roots) > 0 {
		return cloudAutoCategoryTarget{Library: lib, RootID: lib.Roots[0].ID}, nil
	}
	return cloudAutoCategoryTarget{Library: lib}, nil
}

func (s *ScannerService) findCloudAutoCategoryTarget(ctx context.Context, rootLibraryID, provider, displayDir, name, kind string) (*model.Library, *model.Library) {
	if s == nil || s.repo == nil || s.repo.Library == nil {
		return nil, nil
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("list libraries for cloud auto category failed", zap.Error(err))
		}
		return nil, nil
	}
	displayDir = normalizeCloudMountDir(provider, displayDir)
	targetKey, _ := CloudLibraryMergeKey(model.Library{Name: name, Type: kind})
	var target *model.Library
	var existingAuto *model.Library
	for _, lib := range libs {
		info, ok := ParseCloudLibraryMount(lib.Path)
		if ok && info.Provider == provider && normalizeCloudMountDir(provider, info.DisplayDir) == displayDir && CloudLibraryAutoCategory(lib) {
			copy := lib
			existingAuto = &copy
			continue
		}
		if lib.ID == rootLibraryID || CloudLibraryAutoCategory(lib) || !lib.Enabled || targetKey == "" {
			continue
		}
		key, ok := CloudLibraryMergeKey(lib)
		if target == nil && ok && key == targetKey {
			copy := lib
			target = &copy
		}
	}
	return target, existingAuto
}

func (s *ScannerService) ensureCloudLibraryRoot(ctx context.Context, libraryID, name, pathValue string) (*model.LibraryRoot, error) {
	if s == nil || s.repo == nil || s.repo.Library == nil {
		return nil, nil
	}
	roots, err := s.repo.Library.ListRoots(ctx, libraryID)
	if err != nil {
		return nil, err
	}
	targetKey := libraryRootPathKey(pathValue)
	for i := range roots {
		if libraryRootPathKey(roots[i].Path) == targetKey {
			if strings.TrimSpace(roots[i].Name) == "" && strings.TrimSpace(name) != "" {
				_ = s.repo.Library.UpdateRoot(ctx, &roots[i], map[string]any{"name": strings.TrimSpace(name)})
				roots[i].Name = strings.TrimSpace(name)
			}
			return &roots[i], nil
		}
	}
	root := &model.LibraryRoot{
		LibraryID: libraryID,
		Name:      strings.TrimSpace(name),
		Path:      pathValue,
		Enabled:   true,
		SortOrder: len(roots),
	}
	if err := s.repo.Library.CreateRoot(ctx, root); err != nil {
		return nil, err
	}
	return root, nil
}

func (s *ScannerService) migrateCloudAutoCategoryLibrary(ctx context.Context, source, target *model.Library, root *model.LibraryRoot) {
	if s == nil || s.repo == nil || s.repo.DB == nil || source == nil || target == nil || source.ID == "" || target.ID == "" {
		return
	}
	updates := map[string]any{"library_id": target.ID}
	if rootID := libraryRootID(root); rootID != "" {
		updates["library_root_id"] = rootID
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("library_id = ?", source.ID).Updates(updates).Error; err != nil {
		if s.log != nil {
			s.log.Warn("migrate cloud auto category media failed",
				zap.String("from_library_id", source.ID),
				zap.String("to_library_id", target.ID),
				zap.Error(err))
		}
		return
	}
	_ = hardDeleteLibraryRoots(ctx, s.repo.DB, source.ID)
	if err := s.repo.Library.Delete(ctx, source.ID); err != nil && s.log != nil {
		s.log.Warn("remove migrated cloud auto category library failed",
			zap.String("library_id", source.ID),
			zap.Error(err))
	}
}

func (s *ScannerService) cloudScanLibraryScopeIDs(ctx context.Context, lib *model.Library, mount CloudMountInfo) []string {
	if lib == nil {
		return nil
	}
	ids := []string{lib.ID}
	if !cloudRootMountNeedsAutoCategory(mount) || s == nil || s.repo == nil || s.repo.Library == nil {
		return ids
	}
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("list libraries for cloud scan scope failed", zap.String("library_id", lib.ID), zap.Error(err))
		}
		return ids
	}
	for _, candidate := range libs {
		if candidate.ID == lib.ID || !CloudLibraryAutoCategory(candidate) {
			continue
		}
		info, ok := ParseCloudLibraryMount(candidate.Path)
		if ok && info.Provider == mount.Provider {
			ids = appendUniqueLibraryIDs(ids, candidate.ID)
		}
	}
	return ids
}
