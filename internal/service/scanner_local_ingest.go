package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ingestFile upserts a single media file. seenInodes dedups hardlinks within a
// single scan; pass a fresh map for one-off ingests. It mutates res counters.
func (s *ScannerService) ingestFile(ctx context.Context, lib *model.Library, root *model.LibraryRoot, path string, size int64, seenInodes map[string]string, existingMedia map[string]existingLocalMedia, writeBatch *localMediaWriteBatch, res *ScanResult) {
	res.Visited++
	ext := strings.ToLower(filepath.Ext(path))
	cleanPath := filepath.Clean(path)

	fileID, skippedDuplicate := s.recordLocalFileIdentity(ctx, path, seenInodes, existingMedia, res)
	if skippedDuplicate {
		return
	}

	parsedSeason, parsedEpisode := ParseEpisode(path)
	localMeta := s.readLocalScanMetadata(lib, root, path, parsedSeason, parsedEpisode)
	media := s.buildLocalScanMedia(localScanMediaInput{
		lib:           lib,
		root:          root,
		path:          path,
		ext:           ext,
		fileID:        fileID,
		size:          size,
		parsedSeason:  parsedSeason,
		parsedEpisode: parsedEpisode,
		localMeta:     localMeta,
	})
	isNewMedia, skipUnchanged := s.localMediaScanState(localMediaScanStateInput{
		ctx:           ctx,
		path:          path,
		cleanPath:     cleanPath,
		ext:           ext,
		size:          size,
		localMeta:     localMeta,
		incoming:      media,
		existingMedia: existingMedia,
	})
	if skipUnchanged {
		res.Skipped++
		return
	}
	if localMeta != nil {
		res.LocalMetadata++
	}

	s.writeLocalScanMedia(localScanWriteInput{
		ctx:        ctx,
		path:       path,
		media:      media,
		isNewMedia: isNewMedia,
		writeBatch: writeBatch,
		after:      s.localProbeAfter(path, ext),
		res:        res,
	})
}

func (s *ScannerService) recordLocalFileIdentity(ctx context.Context, path string, seenInodes map[string]string, existingMedia map[string]existingLocalMedia, res *ScanResult) (string, bool) {
	fileID, hasID := fileIdentity(path)
	if !hasID {
		return "", false
	}
	if first, ok := seenInodes[fileID]; ok && first != path {
		res.Skipped++
		s.log.Debug("scan skip hardlink duplicate",
			zap.String("path", path), zap.String("primary", first))
		return fileID, true
	}
	if existingMedia == nil {
		if other, ok := s.duplicateByFileID(ctx, fileID, path); ok {
			res.Skipped++
			s.log.Debug("scan skip hardlink duplicate (existing)",
				zap.String("path", path), zap.String("primary", other))
			return fileID, true
		}
	}
	seenInodes[fileID] = path
	return fileID, false
}

func (s *ScannerService) readLocalScanMetadata(lib *model.Library, root *model.LibraryRoot, path string, parsedSeason, parsedEpisode int) *LocalMetadata {
	rootPath := lib.Path
	if root != nil && strings.TrimSpace(root.Path) != "" {
		rootPath = root.Path
	}
	localMeta, err := ReadLocalMetadata(path, rootPath, librarySupportsSeasons(lib) || parsedSeason > 0 || parsedEpisode > 0)
	if err != nil {
		s.log.Warn("read local metadata failed", zap.String("path", path), zap.Error(err))
	}
	return localMeta
}

type localMediaScanStateInput struct {
	ctx           context.Context
	path          string
	cleanPath     string
	ext           string
	size          int64
	localMeta     *LocalMetadata
	incoming      *model.Media
	existingMedia map[string]existingLocalMedia
}

func (s *ScannerService) localMediaScanState(in localMediaScanStateInput) (bool, bool) {
	if in.existingMedia == nil {
		return !s.mediaPathExists(in.ctx, in.path), false
	}
	existing, exists := in.existingMedia[in.cleanPath]
	isNewMedia := !exists
	if exists && in.ext != ".strm" && existing.SizeBytes == in.size && !localMetadataNeedsRefresh(existing, in.localMeta) && !localDerivedMetadataNeedsRefresh(existing, in.incoming) {
		return isNewMedia, true
	}
	return isNewMedia, false
}

type localScanMediaInput struct {
	lib           *model.Library
	root          *model.LibraryRoot
	path          string
	ext           string
	fileID        string
	size          int64
	parsedSeason  int
	parsedEpisode int
	localMeta     *LocalMetadata
}

func (s *ScannerService) buildLocalScanMedia(in localScanMediaInput) *model.Media {
	title, year := CleanQueryWithRecognition(context.Background(), s.repo, in.path)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(in.path), in.ext)
	}

	media := &model.Media{
		LibraryID:     in.lib.ID,
		LibraryRootID: libraryRootID(in.root),
		RelativePath:  localRelativePath(in.path, in.root),
		Title:         title,
		Year:          year,
		Path:          in.path,
		SizeBytes:     in.size,
		Container:     strings.TrimPrefix(in.ext, "."),
		FileID:        in.fileID,
		SeasonNum:     in.parsedSeason,
		EpisodeNum:    in.parsedEpisode,
	}
	if in.ext == ".strm" {
		media.Container = "strm"
		if targetURL, err := readLocalSTRMTarget(in.path); err == nil && targetURL != "" {
			media.STRMURL = targetURL
		} else if err != nil {
			s.log.Debug("read local strm failed", zap.String("path", in.path), zap.Error(err))
		}
	}
	if in.localMeta != nil {
		applyLocalMetadata(media, in.localMeta)
	}
	return media
}

func (s *ScannerService) localProbeAfter(path, ext string) func() {
	if ext == ".strm" || s.probe == nil {
		return nil
	}
	return func() {
		s.queueLocalMediaProbe(path)
	}
}

type localScanWriteInput struct {
	ctx        context.Context
	path       string
	media      *model.Media
	isNewMedia bool
	writeBatch *localMediaWriteBatch
	after      func()
	res        *ScanResult
}

func (s *ScannerService) writeLocalScanMedia(in localScanWriteInput) {
	if in.isNewMedia && in.writeBatch != nil {
		in.writeBatch.AddWithAfter(in.path, in.media, in.after)
		return
	}
	if err := s.repo.Media.Upsert(in.ctx, in.media); err != nil {
		addScanError(in.res, in.path, err)
		s.log.Warn("upsert media failed", zap.String("path", in.path), zap.Error(err))
		return
	}
	if in.after != nil {
		in.after()
	}
	if in.isNewMedia {
		in.res.Added++
	} else {
		in.res.Updated++
	}
	s.publishLocalScanProgress(in.path, in.res)
}

func (s *ScannerService) publishLocalScanProgress(path string, res *ScanResult) {
	s.hub.Publish("scan", map[string]any{
		"library_id": res.LibraryID,
		"path":       path,
		"visited":    res.Visited,
		"added":      res.Added,
		"updated":    res.Updated,
		"probed":     res.Probed,
		"local_meta": res.LocalMetadata,
	})
}

// duplicateByFileID reports an existing media path that shares the given inode
// identity but lives at a different path and still exists on disk.
func (s *ScannerService) duplicateByFileID(ctx context.Context, fileID, path string) (string, bool) {
	if fileID == "" {
		return "", false
	}
	var rows []model.Media
	if err := s.repo.DB.WithContext(ctx).
		Where("file_id = ? AND path <> ?", fileID, path).
		Limit(8).Find(&rows).Error; err != nil {
		return "", false
	}
	for _, r := range rows {
		if r.Path == "" {
			continue
		}
		if _, err := os.Stat(r.Path); err == nil {
			return r.Path, true
		}
	}
	return "", false
}

func (s *ScannerService) mediaPathExists(ctx context.Context, path string) bool {
	var count int64
	err := s.repo.DB.WithContext(ctx).Unscoped().Model(&model.Media{}).
		Where("path = ?", path).Count(&count).Error
	return err == nil && count > 0
}
