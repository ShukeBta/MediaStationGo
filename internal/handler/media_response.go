package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/ShukeBta/MediaStationGo/internal/middleware"
	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/service"
)

// mediaForResponse removes storage implementation details from non-admin API
// responses. Playback and management endpoints continue to resolve media by ID
// on the server; clients do not need filesystem paths or source identities.
func mediaForResponse(c *gin.Context, media model.Media) model.Media {
	if middleware.IsAdmin(c) {
		return media
	}
	media.Path = ""
	media.RelativePath = ""
	media.STRMURL = ""
	media.LibraryPath = ""
	media.DisplayLibraryPath = ""
	media.FileHash = ""
	media.FileID = ""
	return media
}

func mediaSliceForResponse(c *gin.Context, items []model.Media) []model.Media {
	if len(items) == 0 {
		return []model.Media{}
	}
	if middleware.IsAdmin(c) {
		return items
	}
	out := make([]model.Media, len(items))
	for i := range items {
		out[i] = mediaForResponse(c, items[i])
	}
	return out
}

func mediaPointerForResponse(c *gin.Context, media *model.Media) *model.Media {
	if media == nil || middleware.IsAdmin(c) {
		return media
	}
	out := mediaForResponse(c, *media)
	return &out
}

func mediaItemsForResponse(c *gin.Context, items []service.MediaItem) []service.MediaItem {
	if len(items) == 0 {
		return []service.MediaItem{}
	}
	if middleware.IsAdmin(c) {
		return items
	}
	out := make([]service.MediaItem, len(items))
	for i := range items {
		out[i] = items[i]
		out[i].Media = mediaForResponse(c, items[i].Media)
		out[i].Versions = mediaSliceForResponse(c, items[i].Versions)
		out[i].Parts = mediaSliceForResponse(c, items[i].Parts)
	}
	return out
}

func seriesCardsForResponse(c *gin.Context, items []service.SeriesCard) []service.SeriesCard {
	if len(items) == 0 {
		return []service.SeriesCard{}
	}
	if middleware.IsAdmin(c) {
		return items
	}
	out := make([]service.SeriesCard, len(items))
	for i := range items {
		out[i] = items[i]
		out[i].Rep = mediaForResponse(c, items[i].Rep)
		out[i].LinkMedia = mediaForResponse(c, items[i].LinkMedia)
	}
	return out
}

func seriesCardPointerForResponse(c *gin.Context, item *service.SeriesCard) *service.SeriesCard {
	if item == nil || middleware.IsAdmin(c) {
		return item
	}
	out := *item
	out.Rep = mediaForResponse(c, item.Rep)
	out.LinkMedia = mediaForResponse(c, item.LinkMedia)
	return &out
}

func mediaVersionListForResponse(c *gin.Context, result service.MediaVersionList) service.MediaVersionList {
	if middleware.IsAdmin(c) {
		return result
	}
	items := make([]service.MediaVersionItem, len(result.Items))
	for i := range result.Items {
		items[i] = result.Items[i]
		items[i].Media = mediaForResponse(c, result.Items[i].Media)
	}
	result.Items = items
	return result
}

func mediaPartListForResponse(c *gin.Context, result service.MediaPartList) service.MediaPartList {
	if middleware.IsAdmin(c) {
		return result
	}
	items := make([]service.MediaPartItem, len(result.Items))
	for i := range result.Items {
		items[i] = result.Items[i]
		items[i].Media = mediaForResponse(c, result.Items[i].Media)
	}
	result.Items = items
	return result
}

func historyItemsForResponse(c *gin.Context, items []service.HistoryItem) []service.HistoryItem {
	if len(items) == 0 {
		return []service.HistoryItem{}
	}
	if middleware.IsAdmin(c) {
		return items
	}
	out := make([]service.HistoryItem, len(items))
	for i := range items {
		out[i] = items[i]
		if items[i].Media != nil {
			media := mediaForResponse(c, *items[i].Media)
			out[i].Media = &media
		}
	}
	return out
}

func playlistDetailForResponse(c *gin.Context, detail *service.PlaylistDetail) *service.PlaylistDetail {
	if detail == nil || middleware.IsAdmin(c) {
		return detail
	}
	out := *detail
	out.Items = mediaSliceForResponse(c, detail.Items)
	return &out
}

func recycleItemsForResponse(c *gin.Context, items []service.RecycleBinItem) []service.RecycleBinItem {
	if len(items) == 0 {
		return []service.RecycleBinItem{}
	}
	if middleware.IsAdmin(c) {
		return items
	}
	out := make([]service.RecycleBinItem, len(items))
	for i := range items {
		out[i] = items[i]
		out[i].Media = mediaForResponse(c, items[i].Media)
	}
	return out
}
