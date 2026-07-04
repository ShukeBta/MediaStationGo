package service

import (
	"sort"
	"sync"
	"time"
)

type cloudCandidate struct {
	ref                string
	name               string
	size               int64
	path               string
	categoryDisplayDir string
	categoryScanDir    string
	localMeta          *LocalMetadata
}

type cloudScanProgressState struct {
	mu              sync.Mutex
	startedAt       time.Time
	lastProgress    time.Time
	dirsVisited     int
	filesDiscovered int
}

type cloudScanProgressSnapshot struct {
	dirsVisited     int
	filesDiscovered int
	visited         int
	added           int
	updated         int
	skipped         int
	removed         int64
	elapsed         time.Duration
}

func newCloudScanProgressState() *cloudScanProgressState {
	return &cloudScanProgressState{startedAt: time.Now()}
}

func (p *cloudScanProgressState) markDirVisited() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.dirsVisited++
	return p.dirsVisited == 1 || p.dirsVisited%20 == 0
}

func (p *cloudScanProgressState) markFileDiscovered() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.filesDiscovered++
	return p.filesDiscovered%100 == 0
}

func (p *cloudScanProgressState) addSkipped(res *ScanResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	res.Skipped++
}

func (p *cloudScanProgressState) publish(s *ScannerService, libraryID string, res *ScanResult, stage string, force bool) {
	if s == nil || s.hub == nil {
		return
	}
	snap, ok := p.snapshotForProgress(res, force)
	if !ok {
		return
	}
	filesPerSecond := snap.filesPerSecond()
	s.updateCloudScanProgress(libraryID, stage, snap.dirsVisited, snap.filesDiscovered, snap.visited, snap.added, snap.updated, snap.skipped, snap.removed, filesPerSecond)
	s.hub.Publish("scan", map[string]any{
		"library_id":       libraryID,
		"cloud":            true,
		"stage":            stage,
		"dirs":             snap.dirsVisited,
		"discovered":       snap.filesDiscovered,
		"visited":          snap.visited,
		"added":            snap.added,
		"updated":          snap.updated,
		"skipped":          snap.skipped,
		"elapsed_seconds":  int(snap.elapsed.Seconds()),
		"files_per_second": filesPerSecond,
		"estimate_message": "云盘接口不提供总文件数，剩余时间会随目录大小和网盘响应速度变化",
	})
}

func (p *cloudScanProgressState) snapshotForProgress(res *ScanResult, force bool) (cloudScanProgressSnapshot, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !force && time.Since(p.lastProgress) < 2*time.Second {
		return cloudScanProgressSnapshot{}, false
	}
	p.lastProgress = time.Now()
	return p.snapshotLocked(res), true
}

func (p *cloudScanProgressState) finalSnapshot(res *ScanResult) cloudScanProgressSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotLocked(res)
}

func (p *cloudScanProgressState) snapshotLocked(res *ScanResult) cloudScanProgressSnapshot {
	snap := cloudScanProgressSnapshot{
		dirsVisited:     p.dirsVisited,
		filesDiscovered: p.filesDiscovered,
		elapsed:         time.Since(p.startedAt),
	}
	if res != nil {
		snap.visited = res.Visited
		snap.added = res.Added
		snap.updated = res.Updated
		snap.skipped = res.Skipped
		snap.removed = res.Removed
	}
	return snap
}

func (s cloudScanProgressSnapshot) filesPerSecond() float64 {
	processed := s.filesDiscovered
	if s.visited > processed {
		processed = s.visited
	}
	if s.elapsed.Seconds() <= 0 {
		return 0
	}
	return float64(processed) / s.elapsed.Seconds()
}

func publishCloudScanFinished(s *ScannerService, libraryID string, res *ScanResult, progress *cloudScanProgressState) {
	if s == nil || s.hub == nil || progress == nil {
		return
	}
	snap := progress.finalSnapshot(res)
	s.hub.Publish("scan", map[string]any{
		"library_id":      libraryID,
		"finished":        true,
		"visited":         res.Visited,
		"added":           res.Added,
		"updated":         res.Updated,
		"skipped":         res.Skipped,
		"removed":         res.Removed,
		"error_count":     res.ErrorCount,
		"errors":          res.Errors,
		"discovered":      snap.filesDiscovered,
		"dirs":            snap.dirsVisited,
		"elapsed_seconds": int(snap.elapsed.Seconds()),
		"cloud":           true,
	})
}

func sortCloudCandidatesByRefreshPriority(candidates []cloudCandidate, existingMedia map[string]existingCloudMedia) {
	if existingMedia == nil {
		return
	}
	priority := func(candidate cloudCandidate) int {
		existing, ok := existingMedia[candidate.path]
		if !ok {
			return 2
		}
		if cloudTrackMetadataMissing(existing) || cloudMetadataNeedsRefresh(existing, candidate.localMeta) {
			return 0
		}
		return 1
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return priority(candidates[i]) < priority(candidates[j])
	})
}
