package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

func (s *ScannerService) cloudScanWorkerCount() int {
	if s == nil || s.cfg == nil {
		return 4
	}
	return normalizeCloudScanMaxConcurrent(s.cfg.App.CloudScanMaxConcurrent)
}

func normalizeCloudScanMaxConcurrent(n int) int {
	if n <= 0 {
		return 1
	}
	if n > 16 {
		return 16
	}
	return n
}

type cloudScanCandidateRequest struct {
	provider         string
	rootDir          string
	rootDisplayDir   string
	exactFileName    string
	refreshRoot      bool
	refreshDepth     int
	refreshDirs      map[string]struct{}
	strictListErrors bool
	autoCategoryRoot bool
	existingMedia    map[string]existingCloudMedia
	externalMetadata *cloudExternalMetadataCache
	progress         *cloudScanProgressState
	result           *ScanResult
}

type cloudScanCandidateCollection struct {
	candidates []cloudCandidate
	manifest   cloudTreeManifest
}

func (s *ScannerService) collectCloudScanCandidates(ctx context.Context, lib *model.Library, req cloudScanCandidateRequest) ([]cloudCandidate, error) {
	collection, err := s.collectCloudScanCandidateCollection(ctx, lib, req)
	return collection.candidates, err
}

func (s *ScannerService) collectCloudScanCandidateCollection(ctx context.Context, lib *model.Library, req cloudScanCandidateRequest) (cloudScanCandidateCollection, error) {
	collector := newCloudScanCandidateCollector(s, ctx, lib, req)
	return collector.collect()
}

type cloudScanCandidateCollector struct {
	scanner *ScannerService
	ctx     context.Context
	cancel  context.CancelFunc
	lib     *model.Library
	req     cloudScanCandidateRequest

	mu             sync.Mutex
	seenRefs       map[string]struct{}
	visitedDirs    map[string]struct{}
	candidates     []cloudCandidate
	candidateByKey map[string]int

	walkWG      sync.WaitGroup
	walkErr     error
	walkErrOnce sync.Once
	listSlots   chan struct{}
	manifest    *cloudTreeManifestBuilder
	listErrors  []string
	failedDirs  map[string]struct{}
}

func newCloudScanCandidateCollector(s *ScannerService, ctx context.Context, lib *model.Library, req cloudScanCandidateRequest) *cloudScanCandidateCollector {
	walkCtx, cancel := context.WithCancel(ctx)
	return &cloudScanCandidateCollector{
		scanner:        s,
		ctx:            walkCtx,
		cancel:         cancel,
		lib:            lib,
		req:            req,
		seenRefs:       make(map[string]struct{}),
		visitedDirs:    map[string]struct{}{},
		candidates:     make([]cloudCandidate, 0, 256),
		candidateByKey: make(map[string]int),
		listSlots:      make(chan struct{}, s.cloudScanWorkerCount()),
		manifest:       newCloudTreeManifestBuilder(req.rootDisplayDir),
		failedDirs:     make(map[string]struct{}),
	}
}

func (c *cloudScanCandidateCollector) collect() (cloudScanCandidateCollection, error) {
	defer c.cancel()
	c.walkWG.Add(1)
	go func() {
		_ = c.walk(c.req.rootDir, c.req.rootDisplayDir, nil, 0)
	}()
	c.walkWG.Wait()
	if listErr := c.strictListError(); listErr != nil {
		return cloudScanCandidateCollection{manifest: c.manifest.build()}, listErr
	}
	if c.walkErr != nil {
		return cloudScanCandidateCollection{manifest: c.manifest.build()}, c.walkErr
	}
	if err := c.ctx.Err(); err != nil {
		return cloudScanCandidateCollection{manifest: c.manifest.build()}, err
	}
	return cloudScanCandidateCollection{candidates: c.candidates, manifest: c.manifest.build()}, nil
}

func (c *cloudScanCandidateCollector) walk(dirID, displayDir string, inheritedMeta *LocalMetadata, depth int) error {
	defer c.walkWG.Done()
	if err := c.ctx.Err(); err != nil {
		c.setWalkErr(err)
		return err
	}
	if !c.markDirectoryVisited(dirID) {
		return nil
	}
	release, err := c.acquireListSlot()
	if err != nil {
		c.setWalkErr(err)
		return err
	}
	defer release()

	var entries []cloud.FileEntry
	_, refreshExplicitly := c.req.refreshDirs[normalizeCloudMountDir("", dirID)]
	if (c.req.refreshRoot && depth <= c.req.refreshDepth) || refreshExplicitly {
		entries, err = c.scanner.storage.CloudListRefresh(c.ctx, c.req.provider, dirID)
	} else {
		entries, err = c.scanner.storage.CloudList(c.ctx, c.req.provider, dirID)
	}
	if err != nil {
		return c.handleListError(dirID, err)
	}
	c.req.progress.publish(c.scanner, c.lib.ID, c.req.result, "listing", c.req.progress.markDirVisited())
	sidecars := newCloudSidecarSet(c.req.provider, entries)
	dirMeta := c.scanner.cloudDirectoryMetadata(c.ctx, c.req.provider, displayDir, sidecars, inheritedMeta)
	c.scanner.cacheCloudMetadataArtworkNow(c.ctx, dirMeta)
	for _, entry := range entries {
		if err := c.ctx.Err(); err != nil {
			c.setWalkErr(err)
			return err
		}
		c.manifest.add(displayDir, entry)
		if c.req.exactFileName != "" && dirID == c.req.rootDir {
			if entry.IsDir || !strings.EqualFold(strings.TrimSpace(entry.Name), c.req.exactFileName) {
				continue
			}
		}
		if entry.IsDir {
			c.queueChildDirectory(displayDir, entry.Name, entry.ID, dirMeta, depth+1)
			continue
		}
		c.addFileCandidate(displayDir, entry, sidecars, dirMeta)
	}
	return nil
}

func (c *cloudScanCandidateCollector) markDirectoryVisited(dirID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.visitedDirs[dirID]; ok {
		return false
	}
	c.visitedDirs[dirID] = struct{}{}
	return true
}

func (c *cloudScanCandidateCollector) acquireListSlot() (func(), error) {
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case c.listSlots <- struct{}{}:
		if err := c.ctx.Err(); err != nil {
			<-c.listSlots
			return nil, err
		}
		return func() { <-c.listSlots }, nil
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
}

func (c *cloudScanCandidateCollector) handleListError(dirID string, err error) error {
	if c.req.strictListErrors {
		c.recordListError(dirID, err)
		return err
	}
	if dirID != c.req.rootDir && !c.req.strictListErrors {
		c.req.progress.addSkipped(c.req.result)
		c.scanner.log.Warn("skip inaccessible cloud directory",
			zap.String("library_id", c.lib.ID),
			zap.String("provider", c.req.provider),
			zap.String("dir", dirID),
			zap.Error(err))
		return nil
	}
	c.setWalkErr(err)
	return err
}

func (c *cloudScanCandidateCollector) queueChildDirectory(displayDir, entryName, entryID string, dirMeta *LocalMetadata, depth int) {
	if strings.TrimSpace(entryID) == "" {
		if c.req.strictListErrors {
			c.recordListError(joinCloudDisplayPath(displayDir, entryName), fmt.Errorf("cloud directory is missing an entry id"))
		}
		return
	}
	c.walkWG.Add(1)
	go func(childID, childDisplay string, childMeta *LocalMetadata, childDepth int) {
		_ = c.walk(childID, childDisplay, childMeta, childDepth)
	}(entryID, joinCloudDisplayPath(displayDir, entryName), dirMeta, depth)
}

type cloudTreeWalkError struct {
	errors     []string
	failedDirs []string
}

func (e *cloudTreeWalkError) Error() string {
	if e == nil || len(e.errors) == 0 {
		return "cloud tree walk failed"
	}
	return strings.Join(e.errors, "; ")
}

func (e *cloudTreeWalkError) FailedDirs() []string {
	if e == nil {
		return nil
	}
	return append([]string(nil), e.failedDirs...)
}

func (c *cloudScanCandidateCollector) recordListError(dirID string, err error) {
	if err == nil {
		return
	}
	dirID = normalizeCloudMountDir("", dirID)
	c.mu.Lock()
	if errors.Is(err, context.Canceled) && len(c.listErrors) > 0 {
		c.mu.Unlock()
		return
	}
	if _, exists := c.failedDirs[dirID]; exists {
		c.mu.Unlock()
		return
	}
	c.failedDirs[dirID] = struct{}{}
	c.listErrors = append(c.listErrors, fmt.Sprintf("list %s: %v", dirID, err))
	c.mu.Unlock()
	if c.req.strictListErrors {
		c.cancel()
	}
}

func (c *cloudScanCandidateCollector) strictListError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.listErrors) == 0 {
		return nil
	}
	dirs := make([]string, 0, len(c.failedDirs))
	for dirID := range c.failedDirs {
		dirs = append(dirs, dirID)
	}
	sort.Strings(dirs)
	return &cloudTreeWalkError{errors: append([]string(nil), c.listErrors...), failedDirs: dirs}
}

func (c *cloudScanCandidateCollector) addFileCandidate(displayDir string, entry cloud.FileEntry, sidecars cloudSidecarSet, dirMeta *LocalMetadata) {
	ext := strings.ToLower(filepath.Ext(entry.Name))
	if _, ok := videoExtensions[ext]; !ok {
		return
	}
	ref := cloudEntryRef(c.req.provider, entry.ID, entry.PickCode)
	if ref == "" {
		c.req.progress.addSkipped(c.req.result)
		return
	}
	if !c.markRefSeen(ref) {
		c.req.progress.addSkipped(c.req.result)
		return
	}
	c.req.progress.publish(c.scanner, c.lib.ID, c.req.result, "listing", c.req.progress.markFileDiscovered())
	displayPath := joinCloudDisplayPath(displayDir, entry.Name)
	path := cloudMediaPath(c.req.provider, displayPath)
	candidate := cloudCandidate{
		ref:  ref,
		name: entry.Name,
		size: entry.Size,
		path: path,
	}
	if c.req.autoCategoryRoot {
		candidate.categoryDisplayDir, candidate.categoryScanDir = cloudAutoCategoryDirsForMediaPath(path)
		if candidate.categoryDisplayDir != "" {
			displayPath = canonicalCloudAutoCategoryMediaDisplayPath(displayPath, candidate.categoryDisplayDir, candidate.categoryScanDir)
			candidate.path = cloudMediaPath(c.req.provider, displayPath)
		}
	}
	localMeta := c.scanner.cloudFileMetadata(c.ctx, c.req.provider, displayPath, entry.Name, sidecars, dirMeta, librarySupportsSeasons(c.lib))
	if !cloudExistingMetadataSatisfiesExternalEnrich(c.req.existingMedia, candidate.path, entry.Size, c.req.provider, ref, localMeta) {
		localMeta = c.scanner.enrichCloudMetadataFromExternalIDsCached(c.ctx, c.lib, candidate.path, localMeta, c.req.externalMetadata)
	}
	if localMeta != nil {
		c.scanner.cacheCloudMetadataArtworkNow(c.ctx, localMeta)
	}
	candidate.localMeta = localMeta
	c.addCandidate(displayDir, entry, candidate)
}

func canonicalCloudAutoCategoryMediaDisplayPath(displayPath, categoryDisplayDir, categoryScanDir string) string {
	displayPath = strings.Trim(strings.TrimSpace(strings.ReplaceAll(displayPath, "\\", "/")), "/")
	categoryDisplayDir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(categoryDisplayDir, "\\", "/")), "/")
	categoryScanDir = strings.Trim(strings.TrimSpace(strings.ReplaceAll(categoryScanDir, "\\", "/")), "/")
	if displayPath == "" || categoryDisplayDir == "" || categoryScanDir == "" || displayPath == categoryDisplayDir || categoryDisplayDir == categoryScanDir {
		return displayPath
	}
	if displayPath == categoryScanDir {
		return categoryDisplayDir
	}
	prefix := strings.TrimRight(categoryScanDir, "/") + "/"
	if strings.HasPrefix(displayPath, prefix) {
		return strings.TrimRight(categoryDisplayDir, "/") + "/" + strings.TrimPrefix(displayPath, prefix)
	}
	return displayPath
}

func (c *cloudScanCandidateCollector) markRefSeen(ref string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.seenRefs[ref]; ok {
		return false
	}
	c.seenRefs[ref] = struct{}{}
	return true
}

func (c *cloudScanCandidateCollector) addCandidate(displayDir string, entry cloud.FileEntry, candidate cloudCandidate) {
	key := cloudMediaDedupeKey(c.lib, displayDir, entry.Name, entry.Size)
	c.mu.Lock()
	defer c.mu.Unlock()
	if key != "" {
		if prevIndex, ok := c.candidateByKey[key]; ok {
			if candidate.size > c.candidates[prevIndex].size {
				c.candidates[prevIndex] = candidate
			}
			c.req.progress.addSkipped(c.req.result)
			return
		}
		c.candidateByKey[key] = len(c.candidates)
	}
	c.candidates = append(c.candidates, candidate)
}

func (c *cloudScanCandidateCollector) setWalkErr(err error) {
	if err == nil {
		return
	}
	c.walkErrOnce.Do(func() {
		c.walkErr = err
	})
}
