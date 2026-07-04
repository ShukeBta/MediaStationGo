package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type cloudScanImportRequest struct {
	provider      string
	candidates    []cloudCandidate
	existingMedia map[string]existingCloudMedia
	writeBatch    *localMediaWriteBatch
	probeBudget   *int
	defaultRootID string
	progress      *cloudScanProgressState
	result        *ScanResult
}

type cloudScanImportResult struct {
	seen              map[string]struct{}
	touchedLibraryIDs []string
	scopeLibraryIDs   []string
}

type cloudLibraryScanCompletion struct {
	libraryID         string
	touchedLibraryIDs []string
	result            *ScanResult
	progress          *cloudScanProgressState
	autoScrape        bool
}

func (s *ScannerService) scanCloudLibrary(ctx context.Context, lib *model.Library, mount CloudMountInfo, autoScrape bool) (*ScanResult, error) {
	return s.scanCloudLibraryWithRoot(ctx, lib, mount, "", autoScrape)
}

func (s *ScannerService) scanCloudLibraryRoot(ctx context.Context, lib *model.Library, root *model.LibraryRoot, mount CloudMountInfo, autoScrape bool) (*ScanResult, error) {
	return s.scanCloudLibraryWithRoot(ctx, lib, mount, libraryRootID(root), autoScrape)
}

func (s *ScannerService) scanCloudLibraryWithRoot(ctx context.Context, lib *model.Library, mount CloudMountInfo, defaultRootID string, autoScrape bool) (*ScanResult, error) {
	res := &ScanResult{LibraryID: lib.ID}
	if s.storage == nil {
		return res, fmt.Errorf("cloud storage service unavailable")
	}

	cfg, err := s.repo.StorageConfig.Get(ctx, mount.Provider)
	if err != nil || cfg == nil {
		return res, fmt.Errorf("storage config not found: %s", mount.Provider)
	}
	if !cfg.Enabled {
		return res, fmt.Errorf("storage %s is disabled", mount.Provider)
	}
	typ := mount.Provider
	rootDir := mount.ScanDir
	rootDisplayDir := mount.DisplayDir
	autoCategoryRoot := cloudRootMountNeedsAutoCategory(mount)
	scopeIDs := s.cloudScanLibraryScopeIDs(ctx, lib, mount)
	progress := newCloudScanProgressState()
	progress.publish(s, lib.ID, res, "listing", true)
	candidates, err := s.collectCloudScanCandidates(ctx, lib, cloudScanCandidateRequest{
		provider:         typ,
		rootDir:          rootDir,
		rootDisplayDir:   rootDisplayDir,
		autoCategoryRoot: autoCategoryRoot,
		progress:         progress,
		result:           res,
	})
	if err != nil {
		return res, err
	}
	existingMedia, err := s.existingCloudMediaSnapshotForLibraries(ctx, scopeIDs)
	if err != nil {
		s.log.Warn("load existing cloud media snapshot failed", zap.String("library_id", lib.ID), zap.Error(err))
		existingMedia = nil
	}
	sortCloudCandidatesByRefreshPriority(candidates, existingMedia)
	writeBatch := newLocalMediaWriteBatch(s, ctx, res, 100)
	probeBudget := maxCloudMediaProbeQueuePerScan
	imported, err := s.importCloudScanCandidates(ctx, lib, cloudScanImportRequest{
		provider:      typ,
		candidates:    candidates,
		existingMedia: existingMedia,
		writeBatch:    writeBatch,
		probeBudget:   &probeBudget,
		defaultRootID: defaultRootID,
		progress:      progress,
		result:        res,
	})
	if err != nil {
		return res, err
	}
	scopeIDs = appendUniqueLibraryIDs(scopeIDs, imported.scopeLibraryIDs...)
	writeBatch.Flush()
	var removed int64
	if defaultRootID != "" {
		removed, err = s.pruneMissingCloudMediaForRoot(ctx, lib.ID, defaultRootID, imported.seen)
	} else {
		removed, err = s.pruneMissingCloudMediaForLibraries(ctx, scopeIDs, imported.seen)
	}
	if err != nil {
		s.log.Warn("prune missing cloud media failed", zap.String("library_id", lib.ID), zap.Error(err))
	} else {
		res.Removed = removed
	}
	s.completeCloudLibraryScan(ctx, cloudLibraryScanCompletion{
		libraryID:         lib.ID,
		touchedLibraryIDs: imported.touchedLibraryIDs,
		result:            res,
		progress:          progress,
		autoScrape:        autoScrape,
	})
	return res, nil
}

type cloudScanTarget struct {
	lib    *model.Library
	rootID string
}

func (s *ScannerService) importCloudScanCandidates(ctx context.Context, rootLib *model.Library, req cloudScanImportRequest) (cloudScanImportResult, error) {
	imported := cloudScanImportResult{
		seen:              make(map[string]struct{}),
		touchedLibraryIDs: []string{},
		scopeLibraryIDs:   []string{},
	}
	targetLibs := map[string]cloudScanTarget{"": {lib: rootLib, rootID: req.defaultRootID}}
	for _, candidate := range req.candidates {
		select {
		case <-ctx.Done():
			return imported, ctx.Err()
		default:
		}
		target := targetLibs[""]
		if candidate.categoryDisplayDir != "" {
			categoryKey := candidate.categoryDisplayDir + "\x00" + candidate.categoryScanDir
			if cached, ok := targetLibs[categoryKey]; ok {
				target = cached
			} else if categoryTarget, err := s.ensureCloudAutoCategoryTarget(ctx, rootLib, req.provider, candidate.categoryDisplayDir, candidate.categoryScanDir); err == nil && categoryTarget.Library != nil {
				target = cloudScanTarget{lib: categoryTarget.Library, rootID: categoryTarget.RootID}
				targetLibs[categoryKey] = target
				imported.scopeLibraryIDs = appendUniqueLibraryIDs(imported.scopeLibraryIDs, categoryTarget.Library.ID)
			} else if err != nil {
				s.log.Warn("ensure cloud auto category library failed",
					zap.String("library_id", rootLib.ID),
					zap.String("provider", req.provider),
					zap.String("category", candidate.categoryDisplayDir),
					zap.String("scan_dir", candidate.categoryScanDir),
					zap.Error(err))
			}
		}
		targetLib := target.lib
		if targetLib == nil {
			targetLib = rootLib
		}
		imported.touchedLibraryIDs = appendUniqueLibraryIDs(imported.touchedLibraryIDs, targetLib.ID)
		imported.seen[candidate.path] = struct{}{}
		s.ingestCloudFile(ctx, targetLib, target.rootID, req.provider, candidate.ref, candidate.path, candidate.name, candidate.size, candidate.localMeta, req.existingMedia, req.writeBatch, req.probeBudget, req.result)
		req.progress.publish(s, rootLib.ID, req.result, "importing", req.result.Visited == 1 || req.result.Visited%100 == 0)
	}
	return imported, nil
}

func (s *ScannerService) completeCloudLibraryScan(ctx context.Context, req cloudLibraryScanCompletion) {
	publishCloudScanFinished(s, req.libraryID, req.result, req.progress)
	s.invalidateMediaCache(ctx)
	targetIDs := appendUniqueLibraryIDs(req.touchedLibraryIDs, req.libraryID)
	for _, targetID := range targetIDs {
		s.maybeGenerateSTRMAfterScan(targetID)
	}
	if scanHasImportChanges(req.result) && req.autoScrape && s.scraper != nil && s.scraper.AnyEnabled() && s.autoScrapeEnabled(ctx) {
		for _, targetID := range targetIDs {
			s.startAutoScrape(ctx, targetID)
		}
	}
}

func scanHasImportChanges(res *ScanResult) bool {
	return res != nil && (res.Added > 0 || res.Updated > 0 || res.Removed > 0)
}
