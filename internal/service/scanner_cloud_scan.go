package service

import (
	"context"
	"fmt"
	"path"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type cloudScanImportRequest struct {
	provider          string
	candidates        []cloudCandidate
	existingMedia     map[string]existingCloudMedia
	writeBatch        *localMediaWriteBatch
	defaultRootID     string
	progress          *cloudScanProgressState
	result            *ScanResult
	forceSeasonNumber int
}

type cloudScanImportResult struct {
	seen              map[string]struct{}
	touchedLibraryIDs []string
	scopeLibraryIDs   []string
}

type cloudLibraryScanCompletion struct {
	libraryID         string
	provider          string
	touchedLibraryIDs []string
	result            *ScanResult
	progress          *cloudScanProgressState
	autoScrape        bool
}

type cloudScanRootTarget struct {
	scanDir       string
	displayDir    string
	exactFileName string
	resolved      bool
	refreshRoot   bool
}

type cloudIgnoredCandidate struct {
	candidate cloudCandidate
	reason    string
}

type cloudCandidateFilter func([]cloudCandidate) ([]cloudCandidate, []cloudIgnoredCandidate)

func (s *ScannerService) scanCloudLibrary(ctx context.Context, lib *model.Library, mount CloudMountInfo, autoScrape bool) (*ScanResult, error) {
	return s.scanCloudLibraryWithRoot(ctx, lib, mount, "", autoScrape)
}

func (s *ScannerService) scanCloudLibraryRoot(ctx context.Context, lib *model.Library, root *model.LibraryRoot, mount CloudMountInfo, autoScrape bool) (*ScanResult, error) {
	return s.scanCloudLibraryWithRoot(ctx, lib, mount, libraryRootID(root), autoScrape)
}

func (s *ScannerService) scanCloudLibraryRootTargets(ctx context.Context, lib *model.Library, root *model.LibraryRoot, mount CloudMountInfo, targets []cloudScanRootTarget, autoScrape bool) (*ScanResult, error) {
	res, _, err := s.scanCloudLibraryRootTargetsFiltered(ctx, lib, root, mount, targets, autoScrape, nil, 0)
	return res, err
}

func (s *ScannerService) scanCloudLibraryRootTargetsFiltered(ctx context.Context, lib *model.Library, root *model.LibraryRoot, mount CloudMountInfo, targets []cloudScanRootTarget, autoScrape bool, filter cloudCandidateFilter, forceSeasonNumber int) (*ScanResult, []cloudIgnoredCandidate, error) {
	res := &ScanResult{LibraryID: lib.ID}
	if s.storage == nil {
		return res, nil, fmt.Errorf("cloud storage service unavailable")
	}
	if len(targets) == 0 {
		return res, nil, nil
	}

	cfg, err := s.repo.StorageConfig.Get(ctx, mount.Provider)
	if err != nil || cfg == nil {
		return res, nil, fmt.Errorf("storage config not found: %s", mount.Provider)
	}
	if !cfg.Enabled {
		return res, nil, fmt.Errorf("storage %s is disabled", mount.Provider)
	}
	typ := mount.Provider
	autoCategoryRoot := cloudRootMountNeedsAutoCategory(mount) && s.cloudAutoCategoryEnabled(ctx)
	scopeIDs := s.cloudScanLibraryScopeIDs(ctx, lib, mount)
	progress := newCloudScanProgressState()
	progress.publish(s, lib.ID, res, "listing", true)
	existingMedia, err := s.existingCloudMediaSnapshotForLibraries(ctx, scopeIDs)
	if err != nil {
		s.log.Warn("load existing cloud media snapshot failed", zap.String("library_id", lib.ID), zap.Error(err))
		existingMedia = nil
	}
	externalMetadataCache := newCloudExternalMetadataCache()

	candidates := make([]cloudCandidate, 0, len(targets))
	for _, target := range targets {
		if !target.resolved {
			if err := s.warmCloudScanTargetAncestors(ctx, typ, mount.ScanDir, target.scanDir); err != nil {
				return res, nil, err
			}
		}
		targetCandidates, err := s.collectCloudScanCandidates(ctx, lib, cloudScanCandidateRequest{
			provider:         typ,
			rootDir:          target.scanDir,
			rootDisplayDir:   target.displayDir,
			exactFileName:    target.exactFileName,
			refreshRoot:      !target.resolved || target.refreshRoot,
			autoCategoryRoot: autoCategoryRoot,
			existingMedia:    existingMedia,
			externalMetadata: externalMetadataCache,
			progress:         progress,
			result:           res,
		})
		if err != nil {
			return res, nil, err
		}
		candidates = append(candidates, targetCandidates...)
	}
	ignored := []cloudIgnoredCandidate{}
	if filter != nil {
		candidates, ignored = filter(candidates)
		res.Skipped += len(ignored)
	}
	sortCloudCandidatesByRefreshPriority(candidates, existingMedia)
	writeBatch := newLocalMediaWriteBatch(s, ctx, res, 100)
	imported, err := s.importCloudScanCandidates(ctx, lib, cloudScanImportRequest{
		provider:          typ,
		candidates:        candidates,
		existingMedia:     existingMedia,
		writeBatch:        writeBatch,
		defaultRootID:     libraryRootID(root),
		progress:          progress,
		result:            res,
		forceSeasonNumber: forceSeasonNumber,
	})
	if err != nil {
		return res, ignored, err
	}
	scopeIDs = appendUniqueLibraryIDs(scopeIDs, imported.scopeLibraryIDs...)
	writeBatch.Flush()
	s.completeCloudLibraryScan(ctx, cloudLibraryScanCompletion{libraryID: lib.ID, provider: mount.Provider, touchedLibraryIDs: imported.touchedLibraryIDs, result: res, progress: progress, autoScrape: autoScrape})
	return res, ignored, nil
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
	autoCategoryRoot := cloudRootMountNeedsAutoCategory(mount) && s.cloudAutoCategoryEnabled(ctx)
	scopeIDs := s.cloudScanLibraryScopeIDs(ctx, lib, mount)
	progress := newCloudScanProgressState()
	progress.publish(s, lib.ID, res, "listing", true)
	existingMedia, err := s.existingCloudMediaSnapshotForLibraries(ctx, scopeIDs)
	if err != nil {
		s.log.Warn("load existing cloud media snapshot failed", zap.String("library_id", lib.ID), zap.Error(err))
		existingMedia = nil
	}
	candidates, err := s.collectCloudScanCandidates(ctx, lib, cloudScanCandidateRequest{
		provider:         typ,
		rootDir:          rootDir,
		rootDisplayDir:   rootDisplayDir,
		autoCategoryRoot: autoCategoryRoot,
		existingMedia:    existingMedia,
		externalMetadata: newCloudExternalMetadataCache(),
		progress:         progress,
		result:           res,
	})
	if err != nil {
		return res, err
	}
	sortCloudCandidatesByRefreshPriority(candidates, existingMedia)
	writeBatch := newLocalMediaWriteBatch(s, ctx, res, 100)
	imported, err := s.importCloudScanCandidates(ctx, lib, cloudScanImportRequest{
		provider:      typ,
		candidates:    candidates,
		existingMedia: existingMedia,
		writeBatch:    writeBatch,
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
		provider:          typ,
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
		s.ingestCloudFile(ctx, targetLib, target.rootID, req.provider, candidate.ref, candidate.path, candidate.name, candidate.size, candidate.localMeta, req.existingMedia, req.writeBatch, req.result, req.forceSeasonNumber)
		req.progress.publish(s, rootLib.ID, req.result, "importing", req.result.Visited == 1 || req.result.Visited%100 == 0)
	}
	return imported, nil
}

func (s *ScannerService) completeCloudLibraryScan(ctx context.Context, req cloudLibraryScanCompletion) {
	publishCloudScanFinished(s, req.libraryID, req.result, req.progress)
	s.invalidateMediaCache(ctx)
	if scanHasImportChanges(req.result) && s.subtitle != nil {
		s.subtitle.InvalidateCloudDiscovery("", req.provider)
	}
	targetIDs := appendUniqueLibraryIDs(req.touchedLibraryIDs, req.libraryID)
	for _, targetID := range targetIDs {
		s.maybeGenerateSTRMAfterScan(targetID)
		if s.generatedArtwork != nil {
			lib, findErr := s.repo.Library.FindByID(context.WithoutCancel(ctx), targetID)
			if findErr == nil && lib != nil && lib.GenerateArtwork {
				if _, err := s.generatedArtwork.QueueMissingForLibrary(context.WithoutCancel(ctx), targetID); err != nil {
					s.log.Warn("queue generated artwork after cloud scan failed", zap.String("library_id", targetID), zap.Error(err))
				}
			}
		}
	}
	if scanHasImportChanges(req.result) && req.autoScrape && s.scraper != nil && s.scraper.AnyEnabled() && s.autoScrapeEnabled(ctx) {
		for _, targetID := range targetIDs {
			if s.libraryAllowsAutoScrape(ctx, targetID) {
				s.startAutoScrape(ctx, targetID)
			}
		}
	}
}

func scanHasImportChanges(res *ScanResult) bool {
	return res != nil && (res.Added > 0 || res.Updated > 0 || res.Removed > 0)
}

func (s *ScannerService) resolveCloudScanTargetsForOpenListPaths(ctx context.Context, mount CloudMountInfo, values []string) ([]cloudScanRootTarget, error) {
	if s.storage == nil {
		return nil, fmt.Errorf("cloud storage service unavailable")
	}
	rootDisplay := normalizeCloudMountDir(mount.Provider, mount.DisplayDir)
	rootScan := normalizeCloudMountDir(mount.Provider, mount.ScanDir)
	out := make([]cloudScanRootTarget, 0, len(values))
	seen := map[string]struct{}{}
	parentEntries := map[string]map[string]bool{}
	missing := make([]string, 0)
	for _, value := range values {
		targetDisplay := normalizeCloudMountDir(mount.Provider, value)
		if targetDisplay == "" || targetDisplay == rootDisplay || !cloudScanDirSameOrChild(rootDisplay, targetDisplay) {
			continue
		}
		parentDisplay := normalizeCloudMountDir(mount.Provider, path.Dir(targetDisplay))
		parentScan, ok := cloudScanDirForDisplayDir(mount, parentDisplay)
		if !ok {
			continue
		}
		entries, loaded := parentEntries[parentScan]
		if !loaded {
			if err := s.warmCloudScanTargetAncestors(ctx, mount.Provider, rootScan, parentScan); err != nil {
				return nil, fmt.Errorf("warm OpenList target parent %q: %w", parentDisplay, err)
			}
			listed, err := s.storage.CloudListRefresh(ctx, mount.Provider, parentScan)
			if err != nil {
				return nil, fmt.Errorf("list OpenList target parent %q: %w", parentDisplay, err)
			}
			entries = make(map[string]bool, len(listed))
			for _, entry := range listed {
				name := strings.TrimSpace(entry.Name)
				if name != "" {
					entries[name] = entry.IsDir
				}
			}
			parentEntries[parentScan] = entries
		}
		targetName := strings.TrimSpace(path.Base(targetDisplay))
		isDir, found := entries[targetName]
		if !found {
			for name, candidateIsDir := range entries {
				if strings.EqualFold(name, targetName) {
					isDir = candidateIsDir
					found = true
					break
				}
			}
		}
		if !found {
			missing = append(missing, value)
			continue
		}
		target, ok := cloudScanTargetForResolvedOpenListPath(mount, targetDisplay, isDir)
		if !ok {
			continue
		}
		target.resolved = true
		target.refreshRoot = isDir
		key := target.scanDir + "\x00" + target.displayDir + "\x00" + strings.ToLower(target.exactFileName)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, target)
	}
	if len(out) == 0 && len(missing) > 0 {
		return nil, fmt.Errorf("OpenList target not found in parent listing: %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func cloudScanTargetForResolvedOpenListPath(mount CloudMountInfo, value string, isDir bool) (cloudScanRootTarget, bool) {
	rootDisplay := normalizeCloudMountDir(mount.Provider, mount.DisplayDir)
	displayDir := normalizeCloudMountDir(mount.Provider, value)
	if displayDir == "" || displayDir == rootDisplay || !cloudScanDirSameOrChild(rootDisplay, displayDir) {
		return cloudScanRootTarget{}, false
	}
	exactFileName := ""
	if !isDir {
		exactFileName = path.Base(displayDir)
		displayDir = normalizeCloudMountDir(mount.Provider, path.Dir(displayDir))
	}
	scanDir, ok := cloudScanDirForDisplayDir(mount, displayDir)
	if !ok || (displayDir == rootDisplay && exactFileName == "") {
		return cloudScanRootTarget{}, false
	}
	return cloudScanRootTarget{scanDir: scanDir, displayDir: displayDir, exactFileName: exactFileName}, true
}

func cloudScanDirForDisplayDir(mount CloudMountInfo, displayDir string) (string, bool) {
	rootDisplay := normalizeCloudMountDir(mount.Provider, mount.DisplayDir)
	rootScan := normalizeCloudMountDir(mount.Provider, mount.ScanDir)
	displayDir = normalizeCloudMountDir(mount.Provider, displayDir)
	if displayDir == "" || !cloudScanDirSameOrChild(rootDisplay, displayDir) {
		return "", false
	}
	suffix := strings.Trim(strings.TrimPrefix(displayDir, rootDisplay), "/")
	if suffix == "" {
		return rootScan, true
	}
	return strings.Trim(strings.TrimRight(rootScan, "/")+"/"+suffix, "/"), true
}

func (s *ScannerService) warmCloudScanTargetAncestors(ctx context.Context, provider, rootDir, targetDir string) error {
	for _, dir := range cloudScanTargetAncestorDirs(rootDir, targetDir) {
		if _, err := s.storage.CloudListRefresh(ctx, provider, dir); err != nil {
			return err
		}
	}
	return nil
}

func cloudScanTargetAncestorDirs(rootDir, targetDir string) []string {
	rootDir = strings.Trim(normalizeCloudMountDir("", rootDir), "/")
	targetDir = strings.Trim(normalizeCloudMountDir("", targetDir), "/")
	if targetDir == "" || targetDir == rootDir || !cloudScanDirSameOrChild(rootDir, targetDir) {
		return nil
	}
	suffix := strings.Trim(strings.TrimPrefix(targetDir, rootDir), "/")
	if suffix == "" {
		return nil
	}
	parts := strings.Split(suffix, "/")
	ancestors := []string{rootDir}
	current := rootDir
	for _, part := range parts[:len(parts)-1] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		current = strings.Trim(path.Join(current, part), "/")
		ancestors = append(ancestors, current)
	}
	return uniqueCloudScanDirs(ancestors)
}

func cloudScanDirSameOrChild(parent, child string) bool {
	parent = strings.Trim(parent, "/")
	child = strings.Trim(child, "/")
	if parent == "" {
		return child != ""
	}
	return child == parent || strings.HasPrefix(child, parent+"/")
}

func uniqueCloudScanDirs(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		key := strings.ToLower(strings.Trim(value, "/"))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, strings.Trim(value, "/"))
	}
	return out
}
