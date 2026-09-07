package service

import (
	"context"
	"fmt"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// listMediaVisibleGroupedPersisted serves the library page from the indexed
// effective version-key projection. It returns ok=false while a deploy or a
// direct metadata write is still being backfilled; callers then retain the
// existing behavior until the projection is complete.
func (s *MediaService) listMediaVisibleGroupedPersisted(
	ctx context.Context,
	libraryID string,
	page, pageSize int,
	visibility MediaVisibility,
) ([]MediaItem, int64, bool, error) {
	visibility = ExpandMediaVisibilityForMergedCloudLibraries(ctx, s.repo, visibility)
	libraryIDs, err := MergedLibraryIDsForLibrary(ctx, s.repo, libraryID)
	if err != nil {
		return nil, 0, false, err
	}
	filter := repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	}
	complete, err := s.repo.Media.MediaVersionKeysComplete(ctx, libraryIDs, filter)
	if err != nil {
		return nil, 0, false, err
	}
	if !complete {
		return nil, 0, false, nil
	}
	offset := (page - 1) * pageSize
	selected, _, err := s.repo.Media.ListMediaVersionGroupPage(ctx, libraryIDs, filter, offset, pageSize)
	if err != nil {
		return nil, 0, false, err
	}
	total := int64(0)
	if len(selected) > 0 {
		total = selected[0].TotalGroups
	} else {
		total, err = s.repo.Media.CountMediaVersionGroups(ctx, libraryIDs, filter)
		if err != nil {
			return nil, 0, false, err
		}
	}
	if len(selected) == 0 {
		return []MediaItem{}, total, true, nil
	}
	keys := make([]string, len(selected))
	for i := range selected {
		keys[i] = selected[i].Key
	}
	rows, err := s.repo.Media.ListMediaByVersionGroupKeys(ctx, keys, libraryIDs, filter)
	if err != nil {
		return nil, 0, false, err
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
			return nil, 0, false, fmt.Errorf("persisted media version group %q missing selected rows", key)
		}
		pageItems = append(pageItems, item)
	}
	return pageItems, total, true, nil
}
