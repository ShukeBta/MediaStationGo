// Package service — disk usage breakdown.
//
// StorageService aggregates "how much disk does each library use" for
// the React Storage tab. Numbers are computed from the in-DB
// media.size_bytes column so we never hit the disk on the hot path.
package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// StorageService is the read-only aggregator.
type StorageService struct {
	log  *zap.Logger
	repo *repository.Container
}

// NewStorageService is the constructor.
func NewStorageService(log *zap.Logger, repo *repository.Container) *StorageService {
	return &StorageService{log: log, repo: repo}
}

// Breakdown is what /api/storage returns.
type Breakdown struct {
	TotalBytes   int64           `json:"total_bytes"`
	TotalSeconds int64           `json:"total_seconds"`
	ByLibrary    []LibraryUsage  `json:"by_library"`
	ByContainer  []ContainerStat `json:"by_container"`
}

// LibraryUsage is per-library disk + duration totals.
type LibraryUsage struct {
	LibraryID    string `json:"library_id"`
	Name         string `json:"name"`
	Type         string `json:"type"`
	Path         string `json:"path"`
	MediaCount   int64  `json:"media_count"`
	TotalBytes   int64  `json:"total_bytes"`
	TotalSeconds int64  `json:"total_seconds"`
}

// ContainerStat counts media items per container (mp4 / mkv / …).
type ContainerStat struct {
	Container string `json:"container"`
	Count     int64  `json:"count"`
	Bytes     int64  `json:"bytes"`
}

// Compute returns the full breakdown.
func (s *StorageService) Compute(ctx context.Context) (*Breakdown, error) {
	libs, err := s.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	libs = NormalizeCloudLibraryDisplay(libs)
	out := &Breakdown{ByLibrary: make([]LibraryUsage, 0, len(libs))}
	for _, l := range libs {
		var usage LibraryUsage
		usage.LibraryID = l.ID
		usage.Name = l.Name
		usage.Type = l.Type
		usage.Path = l.Path
		row := struct {
			Count   int64
			Size    int64
			Seconds int64
		}{}
		err := s.repo.DB.WithContext(ctx).
			Table("media").
			Where("library_id = ? AND deleted_at IS NULL", l.ID).
			Select("COUNT(*) as count, COALESCE(SUM(size_bytes),0) as size, COALESCE(SUM(duration_sec),0) as seconds").
			Scan(&row).Error
		if err != nil {
			return nil, err
		}
		usage.MediaCount = row.Count
		usage.TotalBytes = row.Size
		usage.TotalSeconds = row.Seconds
		out.TotalBytes += row.Size
		out.TotalSeconds += row.Seconds
		out.ByLibrary = append(out.ByLibrary, usage)
	}

	rows, err := s.containerStats(ctx)
	if err != nil {
		return nil, err
	}
	out.ByContainer = rows
	return out, nil
}

func (s *StorageService) containerStats(ctx context.Context) ([]ContainerStat, error) {
	rows, err := s.repo.DB.WithContext(ctx).
		Table("media").
		Where("deleted_at IS NULL").
		Select("COALESCE(NULLIF(container,''),'unknown') as container, COUNT(*) as count, COALESCE(SUM(size_bytes),0) as bytes").
		Group("container").
		Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ContainerStat{}
	for rows.Next() {
		var c ContainerStat
		if err := rows.Scan(&c.Container, &c.Count, &c.Bytes); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
