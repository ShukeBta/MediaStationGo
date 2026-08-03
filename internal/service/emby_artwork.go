package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// ImageURL returns artwork for a media/series/season item id.
func (e *EmbyService) ImageURL(ctx context.Context, id, imageType string) (string, error) {
	pick := func(primary, backdrop string) string {
		switch strings.ToLower(imageType) {
		case "backdrop", "art":
			if backdrop != "" {
				return backdrop
			}
		}
		if primary != "" {
			return primary
		}
		return backdrop
	}
	if strings.HasPrefix(id, embyVirtualSeasonPrefix) {
		if raw, ok := e.cachedArtworkURL(id, imageType); ok {
			return raw, nil
		}
		if embyWantsPrimaryImage(imageType) {
			if season, ok := e.cachedSeasonGroup(id); ok {
				return e.localThumbnailFromMediaRows(ctx, season.Episodes)
			}
			if season, ok, err := e.findSeasonGroup(ctx, id, ""); err != nil {
				return "", err
			} else if ok {
				return e.localThumbnailFromMediaRows(ctx, season.Episodes)
			}
		}
		return "", nil
	}
	if strings.HasPrefix(id, embyVirtualSeriesPrefix) {
		if raw, ok := e.cachedArtworkURL(id, imageType); ok {
			return raw, nil
		}
		if embyWantsPrimaryImage(imageType) {
			if series, ok := e.cachedSeriesGroup(id); ok {
				return e.localThumbnailFromMediaRows(ctx, series.Episodes)
			}
			if series, ok, err := e.findSeriesGroup(ctx, id, ""); err != nil {
				return "", err
			} else if ok {
				return e.localThumbnailFromMediaRows(ctx, series.Episodes)
			}
		}
		return "", nil
	}
	m, err := e.repo.Media.FindByID(ctx, id)
	if err == nil && m != nil {
		if e.mediaShouldBeEpisode(ctx, m) {
			switch strings.ToLower(imageType) {
			case "backdrop", "art":
				return "", nil
			}
		}
		if raw := pick(e.mediaPrimaryArtwork(ctx, m), e.mediaBackdropArtwork(ctx, m)); raw != "" {
			return raw, nil
		}
		if embyWantsPrimaryImage(imageType) {
			return e.localVideoThumbnail(ctx, m)
		}
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if series, ok, err := e.findSeriesGroup(ctx, id, ""); err != nil {
		return "", err
	} else if ok {
		return pick(series.PosterURL, series.BackdropURL), nil
	}
	return "", nil
}

func (e *EmbyService) mediaPrimaryArtwork(ctx context.Context, m *model.Media) string {
	if m == nil {
		return ""
	}
	if e.mediaShouldBeEpisode(ctx, m) && strings.TrimSpace(m.BackdropURL) != "" {
		return m.BackdropURL
	}
	return m.PosterURL
}

func (e *EmbyService) mediaBackdropArtwork(ctx context.Context, m *model.Media) string {
	if m == nil {
		return ""
	}
	if e.mediaShouldBeEpisode(ctx, m) {
		return ""
	}
	return m.BackdropURL
}
