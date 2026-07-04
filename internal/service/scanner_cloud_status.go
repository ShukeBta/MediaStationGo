package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) beginCloudScan(ctx context.Context, lib *model.Library, mount CloudMountInfo) (context.Context, func(*ScanResult, error), error) {
	if s == nil || lib == nil {
		return ctx, func(*ScanResult, error) {}, nil
	}
	s.cloudScanMu.Lock()
	if s.cloudScans == nil {
		s.cloudScans = make(map[string]*cloudScanEntry)
	}
	if entry := s.cloudScans[lib.ID]; cloudScanBlocksBegin(entry) {
		s.cloudScanMu.Unlock()
		return ctx, nil, ErrCloudScanAlreadyRunning
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.cloudScans[lib.ID] = newCloudScanEntry(lib.ID, mount.Provider, cancel)
	s.cloudScanMu.Unlock()

	finish := func(res *ScanResult, err error) {
		s.finishCloudScan(lib, mount, res, err)
	}
	return runCtx, finish, nil
}

func newCloudScanEntry(libraryID, provider string, cancel context.CancelFunc) *cloudScanEntry {
	now := time.Now()
	return &cloudScanEntry{
		status: CloudScanStatus{
			LibraryID:  libraryID,
			Provider:   provider,
			Stage:      "listing",
			State:      "running",
			StartedAt:  now,
			UpdatedAt:  now,
			ResumeHint: "中断后再次点击扫描会从头遍历，但已入库媒体会去重更新，只补齐缺失项。",
			Estimate:   "小目录通常几十秒；几万文件的大目录可能需要数分钟到数小时，取决于网盘接口速度。",
		},
		cancel: cancel,
	}
}

func (s *ScannerService) finishCloudScan(lib *model.Library, mount CloudMountInfo, res *ScanResult, err error) {
	s.cloudScanMu.Lock()
	defer s.cloudScanMu.Unlock()
	current := s.cloudScans[lib.ID]
	if current == nil {
		return
	}
	applyCloudScanResult(&current.status, res)
	current.status.UpdatedAt = time.Now()
	current.status.FinishedAt = current.status.UpdatedAt
	current.cancel = nil
	applyCloudScanCompletion(&current.status, err)
	s.publishCloudScanFinished(lib.ID, mount.Provider, current.status)
	s.notifyScanFinished(lib, res, err, true)
}

func applyCloudScanResult(status *CloudScanStatus, res *ScanResult) {
	if status == nil || res == nil {
		return
	}
	status.Visited = res.Visited
	status.Added = res.Added
	status.Updated = res.Updated
	status.Skipped = res.Skipped
	status.Removed = res.Removed
	status.ErrorCount = res.ErrorCount
	status.Errors = append([]string(nil), res.Errors...)
}

func applyCloudScanCompletion(status *CloudScanStatus, err error) {
	if status == nil {
		return
	}
	switch {
	case errors.Is(err, context.Canceled):
		status.State = "canceled"
		status.Stage = "canceled"
		status.Error = ""
	case errors.Is(err, context.DeadlineExceeded):
		status.State = "error"
		status.Stage = "error"
		status.Error = "扫描超时：" + err.Error()
	case err != nil:
		status.State = "error"
		status.Stage = "error"
		status.Error = err.Error()
	default:
		status.State = "finished"
		status.Stage = "finished"
		if status.ErrorCount > 0 {
			status.Error = fmt.Sprintf("部分文件入库失败：%d 个，详情见 errors", status.ErrorCount)
		} else {
			status.Error = ""
		}
	}
}

func (s *ScannerService) publishCloudScanFinished(libraryID, provider string, status CloudScanStatus) {
	if s == nil || s.hub == nil {
		return
	}
	s.hub.Publish("scan", map[string]any{
		"library_id":  libraryID,
		"provider":    provider,
		"cloud":       true,
		"finished":    true,
		"state":       status.State,
		"stage":       status.Stage,
		"error":       status.Error,
		"visited":     status.Visited,
		"added":       status.Added,
		"updated":     status.Updated,
		"skipped":     status.Skipped,
		"removed":     status.Removed,
		"error_count": status.ErrorCount,
		"errors":      status.Errors,
	})
}

func (s *ScannerService) updateCloudScanProgress(libraryID, stage string, dirs, discovered, visited, added, updated, skipped int, removed int64, filesPerSecond float64) {
	if s == nil {
		return
	}
	s.cloudScanMu.Lock()
	defer s.cloudScanMu.Unlock()
	entry := s.cloudScans[libraryID]
	if entry == nil {
		return
	}
	entry.status.Stage = stage
	entry.status.UpdatedAt = time.Now()
	entry.status.Dirs = dirs
	entry.status.Discovered = discovered
	entry.status.Visited = visited
	entry.status.Added = added
	entry.status.Updated = updated
	entry.status.Skipped = skipped
	entry.status.Removed = removed
	entry.status.FilesPerSecond = filesPerSecond
}

func (s *ScannerService) acquireCloudScanSlot(ctx context.Context, libraryID string) (func(), error) {
	if s == nil {
		return func() {}, nil
	}
	s.cloudScanMu.Lock()
	if s.cloudSlots == nil {
		s.cloudSlots = make(chan struct{}, 1)
	}
	slots := s.cloudSlots
	if entry := s.cloudScans[libraryID]; entry != nil {
		entry.status.Stage = "queued"
		entry.status.UpdatedAt = time.Now()
	}
	s.cloudScanMu.Unlock()

	select {
	case slots <- struct{}{}:
		s.cloudScanMu.Lock()
		if entry := s.cloudScans[libraryID]; entry != nil && entry.status.State == "running" {
			entry.status.Stage = "listing"
			entry.status.UpdatedAt = time.Now()
		}
		s.cloudScanMu.Unlock()
		return func() { <-slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// CloudScanStatuses returns the current or most recent status per cloud library.
func (s *ScannerService) CloudScanStatuses() []CloudScanStatus {
	if s == nil {
		return nil
	}
	s.cloudScanMu.Lock()
	defer s.cloudScanMu.Unlock()
	out := make([]CloudScanStatus, 0, len(s.cloudScans))
	for _, entry := range s.cloudScans {
		out = append(out, entry.status)
	}
	return out
}

func (s *ScannerService) CancelCloudScan(libraryID string) bool {
	if s == nil || strings.TrimSpace(libraryID) == "" {
		return false
	}
	s.cloudScanMu.Lock()
	defer s.cloudScanMu.Unlock()
	return cancelCloudScanEntry(s.cloudScans[libraryID])
}

func (s *ScannerService) CancelAllCloudScans() int {
	if s == nil {
		return 0
	}
	s.cloudScanMu.Lock()
	defer s.cloudScanMu.Unlock()
	cancelled := 0
	for _, entry := range s.cloudScans {
		if cancelCloudScanEntry(entry) {
			cancelled++
		}
	}
	return cancelled
}

func (s *ScannerService) CancelCloudScansForProvider(provider string) int {
	if s == nil {
		return 0
	}
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return 0
	}
	s.cloudScanMu.Lock()
	defer s.cloudScanMu.Unlock()
	cancelled := 0
	for _, entry := range s.cloudScans {
		if entry == nil || entry.status.Provider != provider {
			continue
		}
		if cancelCloudScanEntry(entry) {
			cancelled++
		}
	}
	return cancelled
}

func cancelCloudScanEntry(entry *cloudScanEntry) bool {
	if !cloudScanActive(entry) {
		return false
	}
	entry.status.State = "canceling"
	entry.status.Stage = "canceling"
	entry.status.UpdatedAt = time.Now()
	if entry.cancel != nil {
		entry.cancel()
		return true
	}
	entry.status.State = "canceled"
	entry.status.Stage = "canceled"
	entry.status.FinishedAt = time.Now()
	return true
}

func cloudScanActive(entry *cloudScanEntry) bool {
	return entry != nil && (entry.status.State == "running" || entry.status.State == "queued" || entry.status.State == "canceling")
}

func cloudScanBlocksBegin(entry *cloudScanEntry) bool {
	return entry != nil && (entry.status.State == "running" || entry.status.State == "canceling")
}
