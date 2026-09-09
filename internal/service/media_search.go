package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// SearchMedia performs a simple LIKE search across titles.
func (s *MediaService) SearchMedia(ctx context.Context, query string, limit int) ([]model.Media, error) {
	return s.SearchMediaVisible(ctx, query, limit, MediaVisibility{IncludeNSFW: true})
}

func (s *MediaService) SearchMediaVisible(ctx context.Context, query string, limit int, visibility MediaVisibility) ([]model.Media, error) {
	if limit <= 0 {
		limit = 50
	} else if limit > maxMediaSearchLimit {
		limit = maxMediaSearchLimit
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	items, err := s.repo.Media.SearchFiltered(ctx, query, limit, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
	if err != nil {
		return nil, err
	}
	s.attachLibraryMetadata(ctx, items)
	return items, nil
}

func (s *MediaService) SearchMediaVisibleGrouped(ctx context.Context, query string, limit int, visibility MediaVisibility) ([]MediaItem, error) {
	if limit <= 0 {
		limit = 50
	} else if limit > maxMediaSearchLimit {
		limit = maxMediaSearchLimit
	}
	items, err := s.SearchMediaVisible(ctx, query, maxMediaSearchLimit, visibility)
	if err != nil {
		return nil, err
	}
	return firstMediaItems(groupMediaVersions(items), limit), nil
}

func (s *MediaService) SearchMediaVisiblePage(ctx context.Context, query string, page, pageSize int, visibility MediaVisibility) ([]model.Media, int64, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > maxMediaSearchPageSize {
		pageSize = maxMediaSearchPageSize
	}
	if page < 1 {
		page = 1
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	items, total, err := s.repo.Media.SearchFilteredPage(ctx, query, (page-1)*pageSize, pageSize, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
	if err != nil {
		return nil, 0, err
	}
	s.attachLibraryMetadata(ctx, items)
	return items, total, nil
}

func (s *MediaService) SearchMediaVisiblePageGrouped(ctx context.Context, query string, page, pageSize int, visibility MediaVisibility) ([]MediaItem, int64, error) {
	page, pageSize = normalizeGroupedMediaPage(page, pageSize)
	items, err := s.SearchMediaVisible(ctx, query, maxMediaSearchLimit, visibility)
	if err != nil {
		return nil, 0, err
	}
	grouped := groupMediaVersions(items)
	return paginateMediaItems(grouped, page, pageSize), int64(len(grouped)), nil
}

func (s *MediaService) SearchMediaVisibleSeriesPage(ctx context.Context, query string, page, pageSize int, visibility MediaVisibility) ([]SeriesCard, int64, error) {
	page, pageSize = normalizeGroupedMediaPage(page, pageSize)
	return s.searchMediaVisibleWorks(ctx, []string{query}, nil, page, pageSize, visibility)
}

// SearchMediaWorkCandidatesVisible is the bounded work-level lookup used by
// ingest duplicate detection. All query variants are evaluated by one SQL
// request and return one representative card per matching work.
func (s *MediaService) SearchMediaWorkCandidatesVisible(ctx context.Context, queries []string, libraryID string, limit int, visibility MediaVisibility) ([]SeriesCard, int64, error) {
	libraryID = strings.TrimSpace(libraryID)
	if libraryID == "" {
		return nil, 0, fmt.Errorf("library_id is required")
	}
	if limit <= 0 {
		limit = 100
	} else if limit > 100 {
		limit = 100
	}
	return s.searchMediaVisibleWorks(ctx, queries, []string{libraryID}, 1, limit, visibility)
}

func (s *MediaService) searchMediaVisibleWorks(
	ctx context.Context,
	queries []string,
	libraryIDs []string,
	page, pageSize int,
	visibility MediaVisibility,
) ([]SeriesCard, int64, error) {
	ctx, err := s.withMediaLibraryMetadata(ctx)
	if err != nil {
		return nil, 0, err
	}
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	filter := repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	}
	load := func(offset, limit int) ([]repository.SeriesCardGroupCandidate, int64, bool, error) {
		return s.repo.Media.SearchPersistedWorkGroupsPage(ctx, queries, libraryIDs, filter, offset, limit)
	}
	resolveComplete := func(candidates []repository.SeriesCardGroupCandidate, total int64, complete bool, loadErr error, offset, limit int) ([]repository.SeriesCardGroupCandidate, int64, error) {
		if loadErr != nil {
			return nil, 0, loadErr
		}
		if complete {
			return candidates, total, nil
		}
		repaired, repairErr := s.repairPersistedSeriesKeys(ctx, libraryIDs, filter)
		if repairErr != nil {
			return nil, 0, repairErr
		}
		candidates, total, complete, loadErr = load(offset, limit)
		if loadErr != nil {
			return nil, 0, loadErr
		}
		if !complete {
			return nil, 0, fmt.Errorf("persisted work search projection remains incomplete after repairing %d series keys", repaired)
		}
		return candidates, total, nil
	}

	if s.directSeriesSQLGroupingSafe(ctx, libraryIDs, filter) {
		offset := (page - 1) * pageSize
		candidates, total, complete, loadErr := load(offset, pageSize)
		candidates, total, err = resolveComplete(candidates, total, complete, loadErr, offset, pageSize)
		if err != nil {
			return nil, 0, err
		}
		cards := s.persistedSeriesCards(ctx, candidates)
		cards, err = s.resolvePersistedSeriesCards(ctx, candidates, cards, filter)
		if err != nil {
			return nil, 0, err
		}
		return s.decorateSeriesCards(ctx, cards), total, nil
	}

	candidates, physicalTotal, complete, loadErr := load(0, maxMediaSearchLimit)
	candidates, physicalTotal, err = resolveComplete(candidates, physicalTotal, complete, loadErr, 0, maxMediaSearchLimit)
	if err != nil {
		return nil, 0, err
	}
	if physicalTotal > maxMediaSearchLimit {
		return nil, 0, fmt.Errorf("work search matched %d physical works, exceeding the exact merge limit %d", physicalTotal, maxMediaSearchLimit)
	}
	cards := s.persistedSeriesCards(ctx, candidates)
	total := int64(len(cards))
	start := (page - 1) * pageSize
	if start >= len(cards) {
		return []SeriesCard{}, total, nil
	}
	end := start + pageSize
	if end > len(cards) {
		end = len(cards)
	}
	cards, err = s.resolvePersistedSeriesCards(ctx, candidates, cards[start:end], filter)
	if err != nil {
		return nil, 0, err
	}
	return s.decorateSeriesCards(ctx, cards), total, nil
}
