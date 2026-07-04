package handler

import (
	"context"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/service"
)

func reclassifyMediaAfterScrape(ctx context.Context, svc *service.Container, mediaIDs ...string) int {
	return reclassifyMediaAfterScrapeWithTypeHints(ctx, svc, nil, mediaIDs...)
}

func reclassifyMediaAfterScrapeWithTypeHints(ctx context.Context, svc *service.Container, mediaTypeHints map[string]string, mediaIDs ...string) int {
	if svc == nil || svc.Organizer == nil {
		return 0
	}
	res, err := svc.Organizer.ReclassifyMisclassifiedMedia(ctx, service.MediaCategoryReclassifyOptions{
		MediaIDs:       mediaIDs,
		MediaTypeHints: mediaTypeHints,
	})
	if err != nil {
		if svc.Log != nil {
			svc.Log.Warn("scrape reclassify media failed", zap.Strings("media_ids", mediaIDs), zap.Error(err))
		}
		return 0
	}
	return res.Reclassified
}

func reclassifyLibraryAfterScrape(ctx context.Context, svc *service.Container, libraryIDs ...string) int {
	if svc == nil || svc.Organizer == nil {
		return 0
	}
	res, err := svc.Organizer.ReclassifyMisclassifiedMedia(ctx, service.MediaCategoryReclassifyOptions{LibraryIDs: libraryIDs})
	if err != nil {
		if svc.Log != nil {
			svc.Log.Warn("scrape reclassify library failed", zap.Strings("library_ids", libraryIDs), zap.Error(err))
		}
		return 0
	}
	return res.Reclassified
}
