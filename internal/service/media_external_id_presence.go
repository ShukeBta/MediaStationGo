package service

import (
	"context"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

// FindVisibleExternalIDPresence checks media presence without exposing media
// metadata, while enforcing the caller's normal media visibility policy.
func (s *MediaService) FindVisibleExternalIDPresence(
	ctx context.Context,
	tmdbIDs []int,
	doubanIDs []string,
	visibility MediaVisibility,
) (map[int]bool, map[string]bool, error) {
	return s.repo.Media.FindExternalIDPresence(ctx, tmdbIDs, doubanIDs, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
}
