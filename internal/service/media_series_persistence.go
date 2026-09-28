package service

import (
	"context"
	"fmt"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// listPersistedSeriesCardGroups returns the only supported source for series
// cards. Unexpected stale keys are repaired in a bounded batch; an incomplete
// projection is then reported explicitly instead of silently loading and
// grouping the entire media table in Go.
func (s *MediaService) listPersistedSeriesCardGroups(
	ctx context.Context,
	libraryIDs []string,
	filter repository.MediaQueryFilter,
) ([]repository.SeriesCardGroupCandidate, error) {
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return nil, fmt.Errorf("media service unavailable")
	}

	groups, complete, err := s.repo.Media.ListPersistedSeriesCardGroups(ctx, libraryIDs, filter)
	if err != nil {
		return nil, err
	}
	if !complete {
		repaired, repairErr := s.repairPersistedSeriesKeys(ctx, libraryIDs, filter)
		if repairErr != nil {
			return nil, repairErr
		}
		groups, complete, err = s.repo.Media.ListPersistedSeriesCardGroups(ctx, libraryIDs, filter)
		if err != nil {
			return nil, err
		}
		if !complete {
			return nil, incompleteSeriesKeysError(repaired)
		}
	}
	return groups, nil
}

func (s *MediaService) repairPersistedSeriesKeys(ctx context.Context, libraryIDs []string, filter repository.MediaQueryFilter) (int64, error) {
	const repairLimit = 500
	s.seriesKeyRepairMu.Lock()
	defer s.seriesKeyRepairMu.Unlock()
	repaired, err := s.repo.Media.BackfillSeriesKeysFiltered(ctx, libraryIDs, filter, repairLimit)
	if err != nil {
		return 0, fmt.Errorf("repair persisted series keys: %w", err)
	}
	return repaired, nil
}

func incompleteSeriesKeysError(repaired int64) error {
	const repairLimit = 500
	return fmt.Errorf("persisted series keys remain incomplete after repairing %d rows (request limit %d)", repaired, repairLimit)
}
