package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScannerService) shadowedCloudLibrary(ctx context.Context, lib *model.Library) *CloudMountConflict {
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		s.log.Warn("list libraries for cloud shadow check failed", zap.String("library_id", lib.ID), zap.Error(err))
		return nil
	}
	visible := FilterScannableCloudLibraries(ctx, s.repo, libs)
	for _, kept := range visible {
		if kept.ID == lib.ID {
			return nil
		}
	}
	current, ok := ParseCloudLibraryMount(lib.Path)
	if ok {
		currentKey, _ := cloudLibraryDisplayKey(*lib)
		for _, kept := range visible {
			info, ok := ParseCloudLibraryMount(kept.Path)
			if !ok || info.Provider != current.Provider {
				continue
			}
			keptKey, _ := cloudLibraryDisplayKey(kept)
			exact := currentKey != "" && currentKey == keptKey
			return &CloudMountConflict{
				Library:            kept,
				Exact:              exact,
				Nested:             !exact,
				ExistingIsAncestor: cloudMountAncestor(info.DisplayDir, current.DisplayDir),
			}
		}
	}
	return CloudLibraryShadowed(libs, *lib)
}
