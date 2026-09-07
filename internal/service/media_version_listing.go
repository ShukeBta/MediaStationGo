package service

import (
	"context"
	"fmt"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// EnsureMediaVersionKeys completes the persisted version-group projection
// before request handling starts. This is an authoritative derived column,
// not a response cache: normal writes maintain it in the same write path.
func (s *MediaService) EnsureMediaVersionKeys(ctx context.Context) (int64, error) {
	return s.ensureMediaVersionKeys(ctx, nil, repository.MediaQueryFilter{IncludeNSFW: true})
}

func (s *MediaService) ensureMediaVersionKeys(
	ctx context.Context,
	libraryIDs []string,
	filter repository.MediaQueryFilter,
) (int64, error) {
	if s == nil || s.repo == nil || s.repo.Media == nil {
		return 0, fmt.Errorf("media version key repository unavailable")
	}
	s.versionKeyRepairMu.Lock()
	defer s.versionKeyRepairMu.Unlock()

	complete, err := s.repo.Media.MediaVersionKeysComplete(ctx, libraryIDs, filter)
	if err != nil {
		return 0, err
	}
	if complete {
		return 0, nil
	}

	var repaired int64
	for {
		n, err := s.repo.Media.BackfillMediaVersionKeysFiltered(ctx, libraryIDs, filter, 10000)
		if err != nil {
			return repaired, err
		}
		repaired += n
		if n == 0 {
			break
		}
	}
	complete, err = s.repo.Media.MediaVersionKeysComplete(ctx, libraryIDs, filter)
	if err != nil {
		return repaired, err
	}
	if !complete {
		return repaired, fmt.Errorf("media version key projection remains incomplete after repairing %d rows", repaired)
	}
	return repaired, nil
}

// listMediaVisibleGroupedPersisted serves the library page only from the
// indexed effective version-key projection. It never falls back to loading
// and grouping every media row in Go.
func (s *MediaService) listMediaVisibleGroupedPersisted(
	ctx context.Context,
	libraryID string,
	page, pageSize int,
	visibility MediaVisibility,
) ([]MediaItem, int64, error) {
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	libraryIDs, err := MergedLibraryIDsForLibrary(ctx, s.repo, libraryID)
	if err != nil {
		return nil, 0, err
	}
	filter := repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	}
	complete, err := s.repo.Media.MediaVersionKeysComplete(ctx, libraryIDs, filter)
	if err != nil {
		return nil, 0, err
	}
	if !complete {
		if _, err := s.ensureMediaVersionKeys(ctx, libraryIDs, filter); err != nil {
			return nil, 0, fmt.Errorf("repair media version keys: %w", err)
		}
	}
	offset := (page - 1) * pageSize
	selected, _, err := s.repo.Media.ListMediaVersionGroupPage(ctx, libraryIDs, filter, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}
	total := int64(0)
	if len(selected) > 0 {
		total = selected[0].TotalGroups
	} else {
		total, err = s.repo.Media.CountMediaVersionGroups(ctx, libraryIDs, filter)
		if err != nil {
			return nil, 0, err
		}
	}
	if len(selected) == 0 {
		return []MediaItem{}, total, nil
	}
	keys := make([]string, len(selected))
	for i := range selected {
		keys[i] = selected[i].Key
	}
	items, err := s.hydrateMediaVersionGroups(ctx, keys, libraryIDs, filter)
	return items, total, err
}

func (s *MediaService) hydrateMediaVersionGroups(ctx context.Context, keys, libraryIDs []string, filter repository.MediaQueryFilter) ([]MediaItem, error) {
	rows, err := s.repo.Media.ListMediaByVersionGroupKeys(ctx, keys, libraryIDs, filter)
	if err != nil {
		return nil, err
	}
	s.attachLibraryMetadata(ctx, rows)
	grouped := groupMediaVersions(rows)
	byKey := make(map[string]MediaItem, len(grouped))
	for i := range grouped {
		byKey[mediaVersionPersistedKey(grouped[i].Media)] = grouped[i]
	}
	pageItems := make([]MediaItem, 0, len(keys))
	for _, key := range keys {
		item, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("persisted media version group %q missing selected rows", key)
		}
		pageItems = append(pageItems, item)
	}
	return pageItems, nil
}
