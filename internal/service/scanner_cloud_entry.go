package service

import (
	"context"
	"errors"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) scanMountedCloudLibrary(ctx context.Context, lib *model.Library, mount CloudMountInfo, autoScrape bool) (*ScanResult, error) {
	if IsDeprecatedNativeCloudProvider(mount.Provider) {
		return &ScanResult{LibraryID: lib.ID, Skipped: 1}, nil
	}
	if CloudLibraryAutoCategory(*lib) {
		res := &ScanResult{LibraryID: lib.ID, Skipped: 1}
		s.log.Info("skip auto category cloud library scan",
			zap.String("library_id", lib.ID),
			zap.String("provider", mount.Provider))
		s.hub.Publish("scan", map[string]any{
			"library_id":    lib.ID,
			"finished":      true,
			"skipped":       res.Skipped,
			"cloud":         true,
			"auto_category": true,
		})
		return res, nil
	}
	if shadow := s.shadowedCloudLibrary(ctx, lib); shadow != nil {
		res := &ScanResult{LibraryID: lib.ID, Skipped: 1}
		s.log.Warn("skip shadowed cloud library scan",
			zap.String("library_id", lib.ID),
			zap.String("shadowed_by", shadow.Library.ID),
			zap.String("provider", mount.Provider))
		s.hub.Publish("scan", map[string]any{
			"library_id": lib.ID,
			"finished":   true,
			"skipped":    res.Skipped,
			"cloud":      true,
			"shadowed":   true,
		})
		return res, nil
	}
	scanCtx, finish, err := s.beginCloudScan(ctx, lib, mount)
	if err != nil {
		if errors.Is(err, ErrCloudScanAlreadyRunning) {
			return &ScanResult{LibraryID: lib.ID, Skipped: 1}, nil
		}
		return nil, err
	}
	release, err := s.acquireCloudScanSlot(scanCtx, lib.ID)
	if err != nil {
		res := &ScanResult{LibraryID: lib.ID}
		if finish != nil {
			finish(res, err)
		}
		return res, err
	}
	defer release()
	res, err := s.scanCloudLibrary(scanCtx, lib, mount, autoScrape)
	if finish != nil {
		finish(res, err)
	}
	return res, err
}
