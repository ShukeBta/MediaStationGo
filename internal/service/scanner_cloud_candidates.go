package service

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service/cloud"
)

type cloudScanCandidateRequest struct {
	provider         string
	rootDir          string
	rootDisplayDir   string
	autoCategoryRoot bool
	progress         *cloudScanProgressState
	result           *ScanResult
}

func (s *ScannerService) collectCloudScanCandidates(ctx context.Context, lib *model.Library, req cloudScanCandidateRequest) ([]cloudCandidate, error) {
	collector := newCloudScanCandidateCollector(s, ctx, lib, req)
	return collector.collect()
}

type cloudScanCandidateCollector struct {
	scanner *ScannerService
	ctx     context.Context
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
}

func newCloudScanCandidateCollector(s *ScannerService, ctx context.Context, lib *model.Library, req cloudScanCandidateRequest) *cloudScanCandidateCollector {
	return &cloudScanCandidateCollector{
		scanner:        s,
		ctx:            ctx,
		lib:            lib,
		req:            req,
		seenRefs:       make(map[string]struct{}),
		visitedDirs:    map[string]struct{}{},
		candidates:     make([]cloudCandidate, 0, 256),
		candidateByKey: make(map[string]int),
		listSlots:      make(chan struct{}, s.cloudScanWorkerCount()),
	}
}

func (c *cloudScanCandidateCollector) collect() ([]cloudCandidate, error) {
	c.walkWG.Add(1)
	go func() {
		_ = c.walk(c.req.rootDir, c.req.rootDisplayDir, nil)
	}()
	c.walkWG.Wait()
	if c.walkErr != nil {
		return nil, c.walkErr
	}
	if err := c.ctx.Err(); err != nil {
		return nil, err
	}
	return c.candidates, nil
}

func (c *cloudScanCandidateCollector) walk(dirID, displayDir string, inheritedMeta *LocalMetadata) error {
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

	entries, err := c.scanner.storage.CloudList(c.ctx, c.req.provider, dirID)
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
		if entry.IsDir {
			c.queueChildDirectory(displayDir, entry.Name, entry.ID, dirMeta)
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
	select {
	case c.listSlots <- struct{}{}:
		return func() { <-c.listSlots }, nil
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
}

func (c *cloudScanCandidateCollector) handleListError(dirID string, err error) error {
	if dirID != c.req.rootDir {
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

func (c *cloudScanCandidateCollector) queueChildDirectory(displayDir, entryName, entryID string, dirMeta *LocalMetadata) {
	if strings.TrimSpace(entryID) == "" {
		return
	}
	c.walkWG.Add(1)
	go func(childID, childDisplay string, childMeta *LocalMetadata) {
		_ = c.walk(childID, childDisplay, childMeta)
	}(entryID, joinCloudDisplayPath(displayDir, entryName), dirMeta)
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
		ref:       ref,
		name:      entry.Name,
		size:      entry.Size,
		path:      path,
	}
	if c.req.autoCategoryRoot {
		candidate.categoryDisplayDir, candidate.categoryScanDir = cloudAutoCategoryDirsForMediaPath(path)
		if candidate.categoryDisplayDir != "" {
			displayPath = canonicalCloudAutoCategoryMediaDisplayPath(displayPath, candidate.categoryDisplayDir, candidate.categoryScanDir)
			candidate.path = cloudMediaPath(c.req.provider, displayPath)
		}
	}
	localMeta := c.scanner.cloudFileMetadata(c.ctx, c.req.provider, displayPath, entry.Name, sidecars, dirMeta, librarySupportsSeasons(c.lib))
	localMeta = c.scanner.enrichCloudMetadataFromExternalIDs(c.ctx, c.lib, candidate.path, localMeta)
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
