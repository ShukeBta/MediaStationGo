package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func (s *ScannerService) StartCloudLibraryScan(libraryID string, autoScrape bool) (CloudScanStatus, bool, error) {
	if s == nil {
		return CloudScanStatus{}, false, errors.New("scanner unavailable")
	}
	lib, err := s.repo.Library.FindByID(context.Background(), libraryID)
	if err != nil {
		return CloudScanStatus{}, false, err
	}
	if lib == nil {
		return CloudScanStatus{}, false, errors.New("library not found")
	}
	mount, ok := ParseCloudLibraryMount(lib.Path)
	if !ok {
		return CloudScanStatus{}, false, errors.New("library is not a cloud mount")
	}
	if IsDeprecatedNativeCloudProvider(mount.Provider) {
		return CloudScanStatus{}, false, fmt.Errorf("cloud provider %q is deprecated; use OpenList or CloudDrive2 bridge", mount.Provider)
	}
	s.cloudScanMu.Lock()
	if entry := s.cloudScans[libraryID]; cloudScanBlocksBegin(entry) {
		status := entry.status
		s.cloudScanMu.Unlock()
		return status, false, nil
	}
	s.cloudScanMu.Unlock()

	go func() {
		ctx, cancel := cloudScanContext(context.Background(), cloudScanTimeout(context.Background(), s.repo, 24*time.Hour))
		defer cancel()
		if autoScrape {
			_, err = s.ScanLibrary(ctx, libraryID)
		} else {
			_, err = s.ScanLibraryWithoutAutoScrape(ctx, libraryID)
		}
		if err != nil && !errors.Is(err, ErrCloudScanAlreadyRunning) && s.log != nil {
			s.log.Warn("cloud library background scan failed", zap.String("library_id", libraryID), zap.Error(err))
		}
	}()
	return newCloudScanEntry(libraryID, mount.Provider, nil).status.withQueuedState(), true, nil
}

func (status CloudScanStatus) withQueuedState() CloudScanStatus {
	status.Stage = "queued"
	status.State = "queued"
	return status
}

func cloudScanContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func cloudScanTimeout(ctx context.Context, repo *repository.Container, fallback time.Duration) time.Duration {
	if repo == nil || repo.Setting == nil {
		return fallback
	}
	value, err := repo.Setting.Get(ctx, "cloud.scan_timeout_hours")
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	hours := parseIntSettingDefault(strings.TrimSpace(value), int(fallback/time.Hour))
	if hours <= 0 {
		return 0
	}
	return time.Duration(hours) * time.Hour
}

func (s *ScannerService) StartAllCloudLibraryScans() ([]CloudScanStatus, error) {
	if s == nil {
		return nil, errors.New("scanner unavailable")
	}
	libs, err := s.repo.Library.List(context.Background())
	if err != nil {
		return nil, err
	}
	libs = FilterScannableCloudLibraries(context.Background(), s.repo, libs)
	statuses := make([]CloudScanStatus, 0, len(libs))
	queue := make([]string, 0, len(libs))
	for _, lib := range libs {
		if !lib.Enabled {
			continue
		}
		mount, ok := ParseCloudLibraryMount(lib.Path)
		if !ok {
			continue
		}
		status, queued := s.queueCloudLibraryScan(lib, mount)
		if queued {
			queue = append(queue, lib.ID)
		}
		statuses = append(statuses, status)
	}
	if len(queue) > 0 {
		go s.runQueuedCloudLibraryScans(queue)
	}
	return statuses, nil
}

func (s *ScannerService) queueCloudLibraryScan(lib model.Library, mount CloudMountInfo) (CloudScanStatus, bool) {
	status := newCloudScanEntry(lib.ID, mount.Provider, nil).status.withQueuedState()
	s.cloudScanMu.Lock()
	defer s.cloudScanMu.Unlock()
	if s.cloudScans == nil {
		s.cloudScans = make(map[string]*cloudScanEntry)
	}
	if entry := s.cloudScans[lib.ID]; cloudScanActive(entry) {
		return entry.status, false
	}
	s.cloudScans[lib.ID] = &cloudScanEntry{status: status}
	return status, true
}

func (s *ScannerService) runQueuedCloudLibraryScans(libraryIDs []string) {
	ctx, cancel := cloudScanContext(context.Background(), cloudScanTimeout(context.Background(), s.repo, 24*time.Hour))
	defer cancel()
	for _, libraryID := range libraryIDs {
		if ctx.Err() != nil {
			return
		}
		if s.cloudScanWasCanceled(libraryID) {
			continue
		}
		if _, err := s.ScanLibrary(ctx, libraryID); err != nil && !errors.Is(err, ErrCloudScanAlreadyRunning) && !errors.Is(err, context.Canceled) && s.log != nil {
			s.log.Warn("cloud library queued scan failed", zap.String("library_id", libraryID), zap.Error(err))
		}
	}
}

func (s *ScannerService) cloudScanWasCanceled(libraryID string) bool {
	s.cloudScanMu.Lock()
	defer s.cloudScanMu.Unlock()
	entry := s.cloudScans[libraryID]
	return entry != nil && entry.status.State == "canceled"
}
