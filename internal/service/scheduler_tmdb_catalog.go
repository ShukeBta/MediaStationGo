package service

import "context"

func (s *SchedulerService) SetTMDbCatalog(catalog *TMDbCatalogService) { s.tmdbCatalog = catalog }

func (s *SchedulerService) jobTMDbEpisodeRecheck(ctx context.Context) error {
	if s.tmdbCatalog == nil {
		return nil
	}
	return s.tmdbCatalog.RunCatalogMaintenance(ctx, false)
}

func (s *SchedulerService) jobTMDbSnapshotBackfill(ctx context.Context) error {
	if s.tmdbCatalog == nil {
		return nil
	}
	return s.tmdbCatalog.RunCatalogMaintenance(ctx, true)
}
