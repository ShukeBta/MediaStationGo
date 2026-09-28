package service

import (
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const (
	adultMediaTypeAV  = "AV"
	adultMediaTypeFC2 = "FC2"
)

// adultMediaDisplayType derives a presentation-only subtype for adult media.
// It does not change the library type, persisted metadata, or storage path.
func adultMediaDisplayType(media *model.Media, mediaType string) string {
	if media == nil || (!media.NSFW && !strings.EqualFold(strings.TrimSpace(mediaType), "adult")) {
		return ""
	}
	if adultMediaIsFC2(media) {
		return adultMediaTypeFC2
	}
	return adultMediaTypeAV
}

func adultMediaIsFC2(media *model.Media) bool {
	if media == nil {
		return false
	}
	for _, value := range []string{
		media.OriginalName,
		media.Title,
		media.RelativePath,
		media.Path,
	} {
		if adultFC2Number(value) != "" {
			return true
		}
	}
	for _, genre := range strings.Split(media.Genres, ",") {
		if strings.EqualFold(strings.TrimSpace(genre), "fd2ppv") {
			return true
		}
	}
	return false
}
