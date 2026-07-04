// Package service — organize an arbitrary source directory (e.g. the download
// directory) into the destination library with dedup + 洗版 (resolution
// replacement).
//
// Unlike OrganizeLibraryWithOptions, which only touches model.Media rows that
// already belong to a registered library, OrganizeDirectory walks the source
// directory on disk directly. This lets operators organize the whole download
// directory (/downloads or a NAS direct-read path configured by the operator)
// even though it is not a registered library.
//
// Two protections requested by operators:
//
//   - 去重：目的地已存在同一媒体时不再从来源整理过去（避免重复 / 多倍占用存储）。
//   - 洗版：若来源分辨率高于目的地已存在的版本，则用高分辨率替换低分辨率。
package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
)

var ErrUnsupportedOrganizeSource = errors.New("source is not a supported video file")

const (
	organizeSkipAlreadyOrganized = "already organized"
	organizeSkipActiveDownload   = "active download"
	organizeSkipDuplicateLibrary = "duplicate in library"
	organizeSkipTargetExists     = "target file exists"
	organizeSkipSampleClip       = "sample/trailer clip"
)

// OrganizeDirectory organizes every video file found under opts.SourcePath into
// the destination root, applying dedup + 洗版 (resolution replacement).
func (o *OrganizerService) OrganizeDirectory(ctx context.Context, opts OrganizeOptions) (*OrganizeResult, error) {
	requestedSource := strings.TrimSpace(o.defaultSourceRoot(ctx, opts.SourcePath))
	if requestedSource == "" {
		return nil, errors.New("source path required")
	}
	source, info, statErr := resolveAccessibleMappedPath(requestedSource)
	if statErr != nil {
		return nil, fmt.Errorf("source directory not accessible: %s", filepath.Clean(requestedSource))
	}
	requestedDest := strings.TrimSpace(o.defaultDestRoot(ctx, opts.DestPath))
	if _, ok := ParseCloudLibraryMount(requestedDest); ok {
		return nil, errors.New("organize destination must be a local writable media directory; enable cloud transfer in external storage when writing to cloud")
	}
	resolvedDest := resolveMappedDestinationPath(requestedDest)
	mediaTypeOverride, mediaCategoryOverride := o.effectiveOrganizeOverrides(opts, resolvedDest)
	dest := normalizeOrganizeDestinationRoot(resolvedDest)
	if dest == "" || dest == "." {
		return nil, errors.New("destination path required")
	}
	if !opts.DryRun {
		if err := ensureOrganizeDestinationWritable(dest); err != nil {
			return nil, err
		}
	}
	mode := o.resolveTransferMode(ctx, opts.TransferMode)
	res := &OrganizeResult{SourcePath: source, DestPath: dest, DryRun: opts.DryRun}
	metadataCache := map[string]*Match{}
	activeDownloads := o.newActiveDownloadGuard(ctx)
	if !info.IsDir() {
		ext := strings.ToLower(filepath.Ext(source))
		if _, ok := videoExtensions[ext]; !ok {
			return nil, fmt.Errorf("%w: %s", ErrUnsupportedOrganizeSource, source)
		}
		if activeDownloads.contains(source) {
			res.Skipped++
			res.Items = append(res.Items, OrganizePreviewItem{Source: source, Action: "skip", Reason: organizeSkipActiveDownload})
			o.logOrganizeDirectoryResult("organize file finished", res, mode)
			return res, nil
		}
		if skipped, reason := shouldSkipOrganizeSourceVideo(source, filepath.Dir(source)); skipped {
			res.Skipped++
			res.Items = append(res.Items, OrganizePreviewItem{Source: source, Action: "skip", Reason: reason})
			o.logOrganizeDirectoryResult("organize file finished", res, mode)
			return res, nil
		}
		if err := o.organizeSourceFile(ctx, organizeSourceFileRequest{
			Source:                source,
			SourceRoot:            filepath.Dir(source),
			DestRoot:              dest,
			Mode:                  mode,
			MediaTypeOverride:     mediaTypeOverride,
			MediaCategoryOverride: mediaCategoryOverride,
			DryRun:                opts.DryRun,
			AllowReplaceExisting:  opts.AllowReplaceExisting,
			MetadataCache:         metadataCache,
			Result:                res,
		}); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", filepath.Base(source), err.Error()))
			res.Items = append(res.Items, OrganizePreviewItem{Source: source, Action: "error", Reason: err.Error()})
		}
		o.logOrganizeDirectoryResult("organize file finished", res, mode)
		return res, nil
	}
	walkErr := walk(source, func(path string, wi walkInfo) error {
		if wi.isDir {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if _, ok := videoExtensions[ext]; !ok {
			return nil
		}
		if activeDownloads.contains(path) {
			res.Skipped++
			res.Items = append(res.Items, OrganizePreviewItem{Source: path, Action: "skip", Reason: organizeSkipActiveDownload})
			return nil
		}
		if skipped, reason := shouldSkipOrganizeSourceVideo(path, source); skipped {
			res.Skipped++
			res.Items = append(res.Items, OrganizePreviewItem{Source: path, Action: "skip", Reason: reason})
			return nil
		}
		if err := o.organizeSourceFile(ctx, organizeSourceFileRequest{
			Source:                path,
			SourceRoot:            source,
			DestRoot:              dest,
			Mode:                  mode,
			MediaTypeOverride:     mediaTypeOverride,
			MediaCategoryOverride: mediaCategoryOverride,
			DryRun:                opts.DryRun,
			AllowReplaceExisting:  opts.AllowReplaceExisting,
			MetadataCache:         metadataCache,
			Result:                res,
		}); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", filepath.Base(path), err.Error()))
			res.Items = append(res.Items, OrganizePreviewItem{Source: path, Action: "error", Reason: err.Error()})
		}
		return nil
	})
	if walkErr != nil {
		return res, walkErr
	}
	o.logOrganizeDirectoryResult("organize directory finished", res, mode)
	return res, nil
}

func (o *OrganizerService) logOrganizeDirectoryResult(message string, res *OrganizeResult, mode TransferMode) {
	if o == nil || o.log == nil || res == nil {
		return
	}
	fields := []zap.Field{
		zap.String("source", res.SourcePath),
		zap.String("dest", res.DestPath),
		zap.String("mode", string(mode)),
		zap.Int("organized", res.Organized),
		zap.Int("replaced", res.Replaced),
		zap.Int("reclassified", res.Reclassified),
		zap.Int("skipped", res.Skipped),
		zap.Int("errors", len(res.Errors)),
		zap.Any("skip_reasons", OrganizeSkipReasonCounts(res)),
	}
	if len(res.Errors) > 0 {
		fields = append(fields, zap.Strings("error_samples", organizeErrorSamples(res.Errors, 5)))
		o.log.Warn(message, fields...)
		return
	}
	o.log.Info(message, fields...)
}
