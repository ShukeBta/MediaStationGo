package service

import (
	"context"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// ListMedia paginates media items inside a library.
func (s *MediaService) ListMedia(ctx context.Context, libraryID string, page, pageSize int) ([]model.Media, int64, error) {
	return s.ListMediaVisible(ctx, libraryID, page, pageSize, MediaVisibility{IncludeNSFW: true})
}

func (s *MediaService) ListMediaVisible(ctx context.Context, libraryID string, page, pageSize int, visibility MediaVisibility) ([]model.Media, int64, error) {
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 2000 {
		pageSize = 2000
	}
	if page < 1 {
		page = 1
	}
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
	cacheKey := s.mediaListCacheKey(libraryID, libraryIDs, page, pageSize, filter)
	var cached mediaListCacheValue
	if s.cache != nil && s.cache.GetJSON(ctx, cacheKey, &cached) {
		s.attachLibraryMetadata(ctx, cached.Items)
		return cached.Items, cached.Total, nil
	}
	items, total, err := s.repo.Media.ListByLibrariesFiltered(ctx, libraryIDs, (page-1)*pageSize, pageSize, filter)
	if err != nil {
		return nil, 0, err
	}
	s.attachLibraryMetadata(ctx, items)
	if s.cache != nil {
		s.cache.SetJSON(ctx, cacheKey, mediaListCacheValue{Items: items, Total: total}, time.Duration(s.mediaCacheTTLSeconds())*time.Second)
	}
	return items, total, nil
}

func (s *MediaService) ListMediaVisibleGrouped(ctx context.Context, libraryID string, page, pageSize int, visibility MediaVisibility) ([]MediaItem, int64, error) {
	page, pageSize = normalizeGroupedMediaPage(page, pageSize)
	var err error
	ctx, err = s.withMediaLibraryMetadata(ctx)
	if err != nil {
		return nil, 0, err
	}
	return s.listMediaVisibleGroupedPersisted(ctx, libraryID, page, pageSize, visibility)
}

// GetMedia returns a single media row.
func (s *MediaService) GetMedia(ctx context.Context, id string) (*model.Media, error) {
	media, err := s.repo.Media.FindByID(ctx, id)
	if err != nil || media == nil {
		return media, err
	}
	items := []model.Media{*media}
	s.attachLibraryMetadata(ctx, items)
	if title := strings.TrimSpace(items[0].PartGroupTitle); title != "" {
		items[0].DisplayTitle = title
	}
	*media = items[0]
	return media, nil
}

// GetMediaByIDs returns active media rows with the same display metadata as GetMedia.
func (s *MediaService) GetMediaByIDs(ctx context.Context, ids []string) ([]model.Media, error) {
	items, err := s.repo.Media.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	s.attachLibraryMetadata(ctx, items)
	for i := range items {
		if title := strings.TrimSpace(items[i].PartGroupTitle); title != "" {
			items[i].DisplayTitle = title
		}
	}
	return items, nil
}
