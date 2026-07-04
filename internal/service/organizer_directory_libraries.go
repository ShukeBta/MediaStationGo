package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (o *OrganizerService) organizeLibraryRootForLayout(ctx context.Context, destRoot, mediaType, category string) (string, bool) {
	lib, ok := o.organizeLibraryForLayout(ctx, destRoot, mediaType, category)
	if !ok {
		return "", false
	}
	return filepath.Clean(lib.Path), true
}

func (o *OrganizerService) organizeLibraryForLayout(ctx context.Context, destRoot, mediaType, category string) (model.Library, bool) {
	if o == nil || o.repo == nil || o.repo.Library == nil {
		return model.Library{}, false
	}
	libraries, err := o.repo.Library.List(ctx)
	if err != nil {
		if o.log != nil {
			o.log.Debug("list libraries for organize target failed", zap.Error(err))
		}
		return model.Library{}, false
	}
	destRoot = normalizeMappedOrganizeDestinationRoot(destRoot)
	mediaType = normalizeOrganizeMediaType(mediaType)
	aliases := o.organizeCategoryAliases(mediaType, category)

	var best model.Library
	bestScore := -1
	bestDepth := -1
	for _, lib := range libraries {
		if !lib.Enabled || strings.TrimSpace(lib.Path) == "" {
			continue
		}
		if _, ok := ParseCloudLibraryMount(lib.Path); ok {
			continue
		}
		if isOrganizeStagingDir(lib.Path) {
			// "手动整理"等暂存库不作为入库目标,避免把媒体留在暂存目录里。
			continue
		}
		lib.Path = resolveMappedDestinationPath(lib.Path)
		scopeScore, inScope := o.organizeLibraryTargetScopeScore(lib.Path, destRoot, mediaType, category)
		if !inScope {
			continue
		}
		categoryMatch := len(aliases) > 0 && libraryMatchesOrganizeCategory(lib, aliases)
		typeScore := organizeLibraryTypeScore(mediaType, lib.Type)
		if len(aliases) > 0 {
			if !categoryMatch {
				continue
			}
		} else if typeScore <= 0 {
			continue
		}
		score := typeScore
		if categoryMatch {
			score += 20
		}
		score += scopeScore
		depth := pathDepth(lib.Path)
		if score > bestScore || (score == bestScore && depth > bestDepth) {
			bestScore = score
			bestDepth = depth
			best = lib
		}
	}
	if strings.TrimSpace(best.Path) == "" {
		return model.Library{}, false
	}
	best.Path = filepath.Clean(best.Path)
	return best, true
}

func (o *OrganizerService) organizeLibraryTargetScopeScore(libPath, destRoot, mediaType, category string) (int, bool) {
	libPath = filepath.Clean(strings.TrimSpace(libPath))
	destRoot = filepath.Clean(strings.TrimSpace(destRoot))
	if destRoot == "" || destRoot == "." {
		return 0, true
	}
	if libPath == "" || libPath == "." {
		return 0, false
	}
	collectionRoot := organizeMediaCollectionRoot(destRoot)
	if collectionRoot != "" && pathWithin(libPath, collectionRoot) && !o.organizeLibraryMatchesExpectedPhysicalRoot(libPath, collectionRoot, mediaType, category) {
		return 0, false
	}
	if pathWithin(libPath, destRoot) || pathWithin(destRoot, libPath) {
		return 8, true
	}
	if _, destCategory := o.mediaTypeForDirectoryCategory(filepath.Base(destRoot)); destCategory == "" {
		if collectionRoot != "" && pathWithin(libPath, collectionRoot) {
			return o.organizeLibraryPhysicalRootScore(libPath, collectionRoot, mediaType, category), true
		}
		return 0, false
	}
	if strings.EqualFold(filepath.Dir(libPath), filepath.Dir(destRoot)) {
		return 4, true
	}
	if collectionRoot != "" && pathWithin(libPath, collectionRoot) {
		return o.organizeLibraryPhysicalRootScore(libPath, collectionRoot, mediaType, category), true
	}
	return 0, false
}

func (o *OrganizerService) organizeLibraryMatchesExpectedPhysicalRoot(libPath, collectionRoot, mediaType, category string) bool {
	if strings.TrimSpace(category) == "" {
		return true
	}
	physicalRoot := o.mediaTypeRootDirForCategory(mediaType, category)
	if physicalRoot == "" {
		physicalRoot = mediaTypeRootDir(mediaType)
	}
	if physicalRoot == "" {
		return true
	}
	return pathHasDirectChild(libPath, collectionRoot, physicalRoot)
}

func (o *OrganizerService) organizeLibraryPhysicalRootScore(libPath, collectionRoot, mediaType, category string) int {
	physicalRoot := o.mediaTypeRootDirForCategory(mediaType, category)
	if physicalRoot == "" {
		physicalRoot = mediaTypeRootDir(mediaType)
	}
	if physicalRoot == "" {
		return 1
	}
	if pathHasDirectChild(libPath, collectionRoot, physicalRoot) {
		return 14
	}
	return 1
}

func organizeMediaCollectionRoot(path string) string {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" || clean == "." {
		return ""
	}
	if isGenericMediaRoot(clean) {
		return clean
	}
	base := filepath.Base(clean)
	parent := filepath.Dir(clean)
	if isPhysicalMediaRootDir(base) {
		return parent
	}
	if isPhysicalMediaRootDir(filepath.Base(parent)) {
		return filepath.Dir(parent)
	}
	return ""
}

func isPhysicalMediaRootDir(name string) bool {
	switch normalizeOrganizeCategoryKey(name) {
	case normalizeOrganizeCategoryKey("电影"), normalizeOrganizeCategoryKey("电视剧"), normalizeOrganizeCategoryKey("动漫"), normalizeOrganizeCategoryKey("成人"):
		return true
	default:
		return false
	}
}

func pathHasDirectChild(path, root, child string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return false
	}
	parts := strings.Split(rel, string(os.PathSeparator))
	if len(parts) == 0 {
		return false
	}
	return strings.EqualFold(parts[0], child)
}

func (o *OrganizerService) ensureOrganizeLibraryForRoot(ctx context.Context, root, mediaType, category string) (model.Library, bool) {
	if o == nil || o.repo == nil || o.repo.Library == nil {
		return model.Library{}, false
	}
	root = resolveMappedDestinationPath(root)
	if root == "" || root == "." {
		return model.Library{}, false
	}
	if _, ok := ParseCloudLibraryMount(root); ok {
		return model.Library{}, false
	}
	libraries, err := o.repo.Library.List(ctx)
	if err != nil {
		if o.log != nil {
			o.log.Debug("list libraries before organize auto-create failed", zap.Error(err))
		}
		return model.Library{}, false
	}
	var containingLibrary model.Library
	hasContainingLibrary := false
	for _, lib := range libraries {
		if !lib.Enabled || strings.TrimSpace(lib.Path) == "" {
			continue
		}
		if _, ok := ParseCloudLibraryMount(lib.Path); ok {
			continue
		}
		lib.Path = resolveMappedDestinationPath(lib.Path)
		if strings.EqualFold(lib.Path, root) {
			return lib, true
		}
		if pathWithin(root, lib.Path) {
			containingLibrary = lib
			hasContainingLibrary = true
		}
	}
	if strings.TrimSpace(category) == "" && hasContainingLibrary {
		return containingLibrary, true
	}
	if !o.autoAddLibraryEnabled(ctx) {
		if o.log != nil {
			o.log.Debug("organize skipped missing library auto-create",
				zap.String("path", root),
				zap.String("media_type", mediaType),
				zap.String("category", category))
		}
		return model.Library{}, false
	}
	name := strings.TrimSpace(category)
	if name == "" {
		name = filepath.Base(root)
	}
	if name == "" || name == "." || name == string(os.PathSeparator) {
		name = organizeLibraryTypeName(mediaType)
	}
	lib := model.Library{
		Name:    name,
		Path:    root,
		Type:    organizeLibraryModelType(mediaType),
		Enabled: true,
	}
	if err := o.repo.Library.CreateWithRoots(ctx, &lib, []model.LibraryRoot{{
		Name:      name,
		Path:      root,
		Enabled:   true,
		SortOrder: 0,
	}}); err != nil {
		if o.log != nil {
			o.log.Warn("organize auto-create library failed",
				zap.String("path", root),
				zap.String("type", lib.Type),
				zap.String("name", lib.Name),
				zap.Error(err))
		}
		return model.Library{}, false
	}
	if o.log != nil {
		o.log.Info("organize auto-created missing library",
			zap.String("path", root),
			zap.String("type", lib.Type),
			zap.String("name", lib.Name))
	}
	return lib, true
}
