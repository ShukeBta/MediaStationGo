package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type organizeMediaRequest struct {
	media         *model.Media
	library       *model.Library
	baseRoot      string
	mediaType     string
	mediaCategory string
	dryRun        bool
	transferMode  TransferMode
}

type organizeMediaDestination struct {
	path      string
	libraryID string
	mediaType string
	category  string
}

func (o *OrganizerService) resolveOrganizeMediaRequest(ctx context.Context, mediaID string, opts OrganizeOptions) (organizeMediaRequest, error) {
	m, err := o.repo.Media.FindByID(ctx, mediaID)
	if err != nil || m == nil {
		return organizeMediaRequest{}, errors.New("media not found")
	}
	lib, err := o.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil || lib == nil {
		return organizeMediaRequest{}, errors.New("library not found")
	}
	if _, ok := ParseCloudLibraryMount(lib.Path); ok {
		return organizeMediaRequest{}, errors.New("local organize cannot use cloud libraries directly; use external storage scan/mount for cloud media or enable cloud transfer to write to cloud")
	}
	requestedBaseRoot := o.resolveBaseRoot(ctx, lib, opts.DestPath)
	mediaType, mediaCategory := o.effectiveOrganizeOverrides(opts, requestedBaseRoot)
	baseRoot := normalizeMappedOrganizeDestinationRoot(requestedBaseRoot)
	if _, ok := ParseCloudLibraryMount(baseRoot); ok {
		return organizeMediaRequest{}, errors.New("organize destination must be a local writable media directory; enable cloud transfer in external storage when writing to cloud")
	}
	if !opts.DryRun {
		if err := ensureOrganizeDestinationWritable(baseRoot); err != nil {
			return organizeMediaRequest{}, err
		}
	}
	if isSeriesLibraryType(lib.Type) {
		if err := o.refreshEpisodeIdentity(m, lib); err != nil {
			return organizeMediaRequest{}, err
		}
	}
	o.refreshOrganizeMediaMetadata(ctx, m, lib, opts.MediaType)
	return organizeMediaRequest{
		media:         m,
		library:       lib,
		baseRoot:      baseRoot,
		mediaType:     mediaType,
		mediaCategory: mediaCategory,
		dryRun:        opts.DryRun,
		transferMode:  o.resolveTransferMode(ctx, opts.TransferMode),
	}, nil
}

func (o *OrganizerService) buildOrganizeMediaDestination(ctx context.Context, req organizeMediaRequest) (organizeMediaDestination, error) {
	m := req.media
	lib := req.library
	title := sanitizeFilename(m.Title)
	if title == "" {
		title = "Unknown"
	}

	mediaType := normalizeOrganizeMediaType(req.mediaType)
	if mediaType == "" {
		mediaType = normalizeOrganizeMediaType(lib.Type)
	}
	category := sanitizeFilename(strings.TrimSpace(req.mediaCategory))
	if category == "" && o.isSmartClassifyEnabled(ctx) {
		category = o.classifyMedia(ctx, m, mediaType)
	}
	if impliedType, normalizedCategory := o.mediaTypeForDirectoryCategory(category); impliedType != "" {
		category = normalizedCategory
		if normalizeOrganizeMediaType(req.mediaType) == "" {
			mediaType = impliedType
		}
	}
	root := o.organizeRoot(req.baseRoot, mediaType, category)
	targetLibraryID := ""
	matchedLibrary := false
	if category != "" {
		targetLibrary, ok := o.organizeLibraryForLayout(ctx, req.baseRoot, mediaType, category)
		if ok {
			root = targetLibrary.Path
			targetLibraryID = targetLibrary.ID
			matchedLibrary = true
		}
	}
	if !matchedLibrary && category != "" {
		root = categoryRoot(root, category)
	}
	if !matchedLibrary && !req.dryRun {
		if targetLibrary, ok := o.ensureOrganizeLibraryForRoot(ctx, root, mediaType, category); ok {
			targetLibraryID = targetLibrary.ID
		}
	}
	target, err := o.buildOrganizeTargetPath(ctx, organizeTargetInput{
		Root:      root,
		MediaType: mediaType,
		Category:  category,
		Title:     title,
		Source:    m.Path,
		Ext:       filepath.Ext(m.Path),
		Year:      m.Year,
		Season:    m.SeasonNum,
		Episode:   m.EpisodeNum,
		Series:    isSeriesLibraryType(mediaType),
	})
	if err != nil {
		return organizeMediaDestination{}, err
	}
	return organizeMediaDestination{path: target.Path, libraryID: targetLibraryID, mediaType: mediaType, category: category}, nil
}

func (o *OrganizerService) applyOrganizeMedia(ctx context.Context, req organizeMediaRequest, dst organizeMediaDestination) (string, error) {
	m := req.media

	// Refuse to overwrite an existing different file. 当多个 release（如
	// 不同字幕组、不同源）刮削后被统一改名，原本不重复的文件会被映射到
	// 同一个目标路径，盲目 move 会导致后者覆盖前者，造成数据丢失。
	if _, err := os.Stat(dst.path); err == nil {
		o.log.Warn("organize skipped: destination already exists",
			zap.String("media", m.ID),
			zap.String("from", m.Path),
			zap.String("to", dst.path))
		return dst.path, nil
	}

	if err := os.MkdirAll(filepath.Dir(dst.path), 0o755); err != nil { // #nosec G301 -- organized media directories must remain readable by NAS/player users.
		return "", err
	}
	if err := transferFile(m.Path, dst.path, req.transferMode); err != nil {
		return "", err
	}

	updates := map[string]any{
		"path": dst.path,
	}
	addOrganizedMediaMetadataUpdates(updates, *m)
	if normalizeOrganizeMediaType(dst.mediaType) == "movie" {
		updates["season_num"] = 0
		updates["episode_num"] = 0
	} else {
		updates["season_num"] = m.SeasonNum
		updates["episode_num"] = m.EpisodeNum
	}
	if strings.TrimSpace(dst.libraryID) != "" {
		updates["library_id"] = strings.TrimSpace(dst.libraryID)
	}
	if err := o.repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Where("id = ?", m.ID).
		Updates(updates).Error; err != nil {
		return dst.path, err
	}
	if err := transferSidecarNFO(m.Path, dst.path, req.transferMode); err != nil {
		o.log.Warn("organize sidecar nfo failed",
			zap.String("media", m.ID),
			zap.String("from", nfoPath(m.Path)),
			zap.String("to", nfoPath(dst.path)),
			zap.Error(err))
	}
	o.log.Info("organized",
		zap.String("media", m.ID),
		zap.String("from", m.Path),
		zap.String("to", dst.path),
		zap.String("category", dst.category),
		zap.String("media_type", dst.mediaType),
		zap.String("mode", string(req.transferMode)),
	)
	return dst.path, nil
}

func cleanupEmptyOrganizeParents(oldPath, stopRoot string) {
	dir := filepath.Clean(filepath.Dir(strings.TrimSpace(oldPath)))
	root := filepath.Clean(strings.TrimSpace(stopRoot))
	if dir == "" || root == "" || dir == "." || root == "." || !pathWithin(dir, root) {
		return
	}
	for !samePath(dir, root) {
		if err := os.Remove(dir); err != nil {
			return
		}
		next := filepath.Dir(dir)
		if samePath(next, dir) {
			return
		}
		dir = next
	}
}
