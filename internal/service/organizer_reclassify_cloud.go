package service

import (
	"context"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (o *OrganizerService) reclassifyCloudScannedMedia(ctx context.Context, media model.Media, lib model.Library, mount CloudMountInfo, mediaTypeHint string, dryRun bool, res *OrganizeResult) (bool, error) {
	mediaType := normalizeOrganizeMediaType(lib.Type)
	if mediaTypeHint != "" {
		mediaType = mediaTypeHint
	}
	metadataMatch := organizeMatchFromMedia(&media)
	if metadataMatch != nil && mediaTypeHint != "" {
		metadataMatch.MediaType = mediaTypeHint
	}
	if !mediaHasReliableCategoryMetadata(media) {
		metadataMatch = o.lookupReclassifyMetadata(ctx, media, lib, mediaType)
		if metadataMatch == nil {
			return false, nil
		}
		media = mediaWithReclassifyMatch(media, metadataMatch)
	}
	if matchType := normalizeOrganizeMediaType(metadataMatchMediaType(metadataMatch)); matchType != "" {
		mediaType = matchType
	}
	category := o.classifyMedia(ctx, &media, mediaType)
	if category == "" {
		return false, nil
	}
	if impliedType, normalizedCategory := o.mediaTypeForDirectoryCategory(category); impliedType != "" {
		mediaType = impliedType
		category = normalizedCategory
	}
	if mediaType == "" {
		mediaType = normalizeOrganizeMediaType(lib.Type)
	}
	displayDir := o.cloudReclassifyCategoryDisplayDir(mediaType, category)
	if displayDir == "" {
		return false, nil
	}
	if normalizeCloudMountDir(mount.Provider, mount.DisplayDir) == normalizeCloudMountDir(mount.Provider, displayDir) {
		return false, nil
	}
	targetLibrary, ok, err := o.ensureCloudReclassifyLibrary(ctx, mount.Provider, displayDir, mediaType, dryRun)
	if err != nil || !ok {
		return false, err
	}
	if strings.TrimSpace(targetLibrary.ID) != "" && targetLibrary.ID == lib.ID {
		return false, nil
	}
	title := sanitizeFilename(strings.TrimSpace(media.Title))
	if title == "" {
		title = "Unknown"
	}
	res.Items = append(res.Items, OrganizePreviewItem{
		Source:    media.Path,
		Target:    targetLibrary.Path,
		Action:    "reclassify",
		Reason:    "cloud metadata category library changed",
		MediaType: mediaType,
		Category:  category,
		Title:     title,
	})
	if dryRun {
		res.Reclassified++
		return true, nil
	}
	updates := map[string]any{
		"library_id": targetLibrary.ID,
		"series_id":  "",
	}
	if normalizeOrganizeMediaType(mediaType) == "movie" {
		updates["season_num"] = 0
		updates["episode_num"] = 0
		updates["episode_title"] = ""
	}
	applyReclassifyMatchUpdates(updates, metadataMatch)
	if err := o.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", media.ID).Updates(updates).Error; err != nil {
		return false, err
	}
	if o.log != nil {
		o.log.Info("cloud media library reclassified by metadata",
			zap.String("media", media.ID),
			zap.String("path", media.Path),
			zap.String("from_library", lib.ID),
			zap.String("to_library", targetLibrary.ID),
			zap.String("category", category),
			zap.String("media_type", mediaType),
			zap.String("display_dir", displayDir))
	}
	res.Reclassified++
	return true, nil
}

func (o *OrganizerService) cloudReclassifyCategoryDisplayDir(mediaType, category string) string {
	category = sanitizeFilename(strings.TrimSpace(category))
	if category == "" {
		return ""
	}
	root := o.mediaTypeRootDirForCategory(mediaType, category)
	if root == "" {
		return ""
	}
	return strings.Join([]string{root, category}, "/")
}

func (o *OrganizerService) ensureCloudReclassifyLibrary(ctx context.Context, provider, displayDir, mediaType string, dryRun bool) (model.Library, bool, error) {
	if o == nil || o.repo == nil || o.repo.Library == nil {
		return model.Library{}, false, nil
	}
	provider = strings.TrimSpace(provider)
	displayDir = normalizeCloudMountDir(provider, displayDir)
	if provider == "" || displayDir == "" {
		return model.Library{}, false, nil
	}
	if existing := o.findCloudReclassifyLibrary(ctx, provider, displayDir); existing != nil {
		return *existing, true, nil
	}
	path := BuildCloudAutoCategoryLibraryPath(provider, displayDir)
	if path == "" {
		return model.Library{}, false, nil
	}
	name := cloudMountDirBase(displayDir)
	if name == "" {
		name = displayDir
	}
	libType := InferCloudMountMediaType(displayDir, name)
	if normalizeOrganizeMediaType(libType) == "" {
		libType = organizeLibraryModelType(mediaType)
	}
	lib := model.Library{
		Name:    name,
		Path:    path,
		Type:    libType,
		Enabled: true,
	}
	if dryRun {
		return lib, true, nil
	}
	if err := o.repo.Library.Create(ctx, &lib); err != nil {
		if existing := o.findCloudReclassifyLibrary(ctx, provider, displayDir); existing != nil {
			return *existing, true, nil
		}
		return model.Library{}, false, err
	}
	if o.log != nil {
		o.log.Info("created cloud metadata reclassify library",
			zap.String("library_id", lib.ID),
			zap.String("provider", provider),
			zap.String("display_dir", displayDir),
			zap.String("type", lib.Type))
	}
	return lib, true, nil
}

func (o *OrganizerService) findCloudReclassifyLibrary(ctx context.Context, provider, displayDir string) *model.Library {
	if o == nil || o.repo == nil || o.repo.Library == nil {
		return nil
	}
	libs, err := o.repo.Library.List(ctx)
	if err != nil {
		if o.log != nil {
			o.log.Warn("list cloud libraries for metadata reclassify failed", zap.Error(err))
		}
		return nil
	}
	displayDir = normalizeCloudMountDir(provider, displayDir)
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		info, ok := ParseCloudLibraryMount(lib.Path)
		if !ok || info.Provider != provider {
			continue
		}
		if normalizeCloudMountDir(provider, info.DisplayDir) == displayDir {
			return &lib
		}
	}
	return nil
}
