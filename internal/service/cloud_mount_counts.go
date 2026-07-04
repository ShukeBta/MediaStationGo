package service

import (
	"context"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func cloudLibraryMediaCounts(ctx context.Context, repo *repository.Container, libs []model.Library) map[string]int64 {
	counts := make(map[string]int64, len(libs))
	if repo == nil || repo.DB == nil || len(libs) == 0 {
		return counts
	}
	ids := make([]string, 0, len(libs))
	for _, lib := range libs {
		ids = append(ids, lib.ID)
	}
	if len(ids) == 0 {
		return counts
	}
	var rows []struct {
		LibraryID string
		Count     int64
	}
	if err := repo.DB.WithContext(ctx).
		Model(&model.Media{}).
		Select("library_id, COUNT(*) AS count").
		Where("library_id IN ? AND deleted_at IS NULL", ids).
		Group("library_id").
		Scan(&rows).Error; err != nil {
		return counts
	}
	for _, row := range rows {
		counts[row.LibraryID] = row.Count
	}
	return counts
}
