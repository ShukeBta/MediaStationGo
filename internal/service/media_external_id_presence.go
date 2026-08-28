package service

import (
	"context"

	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

type ExternalIDReference = repository.ExternalIDReference

// FindVisibleExternalIDPresence checks media presence without exposing media
// metadata, while enforcing the caller's normal media visibility policy.
func (s *MediaService) FindVisibleExternalIDPresence(
	ctx context.Context,
	tmdbRefs []ExternalIDReference,
	doubanIDs []string,
	visibility MediaVisibility,
) (map[ExternalIDReference]bool, map[string]bool, error) {
	return s.repo.Media.FindExternalIDPresence(ctx, tmdbRefs, doubanIDs, repository.MediaQueryFilter{
		IncludeNSFW:       visibility.IncludeNSFW,
		AllowedLibraryIDs: visibility.AllowedLibraryIDs,
		HiddenLibraryIDs:  visibility.HiddenLibraryIDs,
	})
}
