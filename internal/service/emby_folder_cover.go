package service

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const embyFolderCoverTagVersion = "folder-cover-grid-v6-jellyfin-shape"

type EmbyFolderCoverArtwork struct {
	MediaID   string
	ImageType string
	Tag       string
	URL       string
}

func (e *EmbyService) FolderCoverArtwork(ctx context.Context, id, imageType string, limit int) ([]EmbyFolderCoverArtwork, error) {
	if e == nil || e.repo == nil || e.repo.Library == nil || e.repo.DB == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 8 {
		limit = 4
	}
	lib, err := e.repo.Library.FindByID(ctx, id)
	if err != nil || lib == nil {
		return nil, err
	}
	libraryIDs := e.mergedLibraryIDs(ctx, lib.ID)
	if len(libraryIDs) == 0 {
		libraryIDs = []string{lib.ID}
	}
	if embyLibraryTypeIsEpisodic(lib.Type) {
		return e.folderCoverSeriesArtwork(ctx, libraryIDs, imageType, limit)
	}
	var rows []model.Media
	if err := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ? AND deleted_at IS NULL", libraryIDs).
		Where("(poster_url <> '' OR backdrop_url <> '' OR generated_poster_url <> '' OR generated_backdrop_url <> '')").
		Order("updated_at DESC, created_at DESC, id DESC").
		Limit(limit * 4).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]EmbyFolderCoverArtwork, 0, limit)
	seenMediaIDs := map[string]struct{}{}
	seenArtworkURLs := map[string]struct{}{}
	for _, preferredType := range folderCoverImageTypePreference(imageType) {
		for i := range rows {
			artwork, ok := folderCoverArtworkForMedia(&rows[i], preferredType)
			if !ok {
				continue
			}
			if _, ok := seenMediaIDs[artwork.MediaID]; ok {
				continue
			}
			if _, ok := seenArtworkURLs[artwork.URL]; ok {
				continue
			}
			seenMediaIDs[artwork.MediaID] = struct{}{}
			seenArtworkURLs[artwork.URL] = struct{}{}
			out = append(out, artwork)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func (e *EmbyService) folderCoverSeriesArtwork(ctx context.Context, libraryIDs []string, imageType string, limit int) ([]EmbyFolderCoverArtwork, error) {
	var rows []model.Media
	if err := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id IN ? AND deleted_at IS NULL", libraryIDs).
		Where("(poster_url <> '' OR backdrop_url <> '')").
		Order("updated_at DESC, created_at DESC, id DESC").
		Limit(embySeriesGroupingLimit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	groups, err := e.seriesGroupsFromMedia(ctx, rows)
	if err != nil {
		return nil, err
	}
	out := make([]EmbyFolderCoverArtwork, 0, limit)
	seenSeriesIDs := map[string]struct{}{}
	seenArtworkURLs := map[string]struct{}{}
	for _, preferredType := range folderCoverImageTypePreference(imageType) {
		for i := range groups {
			artwork, ok := folderCoverArtworkForSeries(&groups[i], preferredType)
			if !ok {
				continue
			}
			if _, ok := seenSeriesIDs[artwork.MediaID]; ok {
				continue
			}
			if _, ok := seenArtworkURLs[artwork.URL]; ok {
				continue
			}
			seenSeriesIDs[artwork.MediaID] = struct{}{}
			seenArtworkURLs[artwork.URL] = struct{}{}
			out = append(out, artwork)
			if len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func (e *EmbyService) FolderCoverTag(ctx context.Context, id, imageType string) string {
	artworks, err := e.FolderCoverArtwork(ctx, id, imageType, 4)
	if err != nil || len(artworks) == 0 {
		return ""
	}
	return EmbyFolderCoverTag(id, artworks)
}

func EmbyFolderCoverTag(folderID string, covers []EmbyFolderCoverArtwork) string {
	items := make([][]string, 0, len(covers))
	for _, cover := range covers {
		items = append(items, []string{cover.MediaID, cover.ImageType, cover.Tag})
	}
	payload := []any{embyFolderCoverTagVersion, strings.TrimSpace(folderID), items}
	body, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha1.Sum(body)
	return hex.EncodeToString(sum[:])[:32]
}

func folderCoverArtworkForMedia(m *model.Media, imageType string) (EmbyFolderCoverArtwork, bool) {
	if m == nil {
		return EmbyFolderCoverArtwork{}, false
	}
	poster := strings.TrimSpace(m.PosterURL)
	backdrop := strings.TrimSpace(m.BackdropURL)
	generatedPoster := false
	generatedBackdrop := false
	if poster == "" {
		poster = strings.TrimSpace(m.GeneratedPosterURL)
		generatedPoster = poster != ""
	}
	if backdrop == "" {
		backdrop = strings.TrimSpace(m.GeneratedBackdropURL)
		generatedBackdrop = backdrop != ""
	}
	switch folderCoverImageTypeValue(imageType) {
	case "Backdrop":
		if backdrop != "" {
			tag := m.ID + "-bd"
			if generatedBackdrop {
				tag = embyImageTag(m.ID, "backdrop", backdrop, m.UpdatedAt)
			}
			return EmbyFolderCoverArtwork{MediaID: m.ID, ImageType: "Backdrop", Tag: tag, URL: backdrop}, true
		}
		return folderCoverArtworkForMedia(m, "Primary")
	case "Thumb":
		return folderCoverArtworkForMedia(m, "Primary")
	default:
		if poster != "" {
			tag := m.ID
			if generatedPoster {
				tag = embyImageTag(m.ID, "primary", poster, m.UpdatedAt)
			}
			return EmbyFolderCoverArtwork{MediaID: m.ID, ImageType: "Primary", Tag: tag, URL: poster}, true
		}
		return EmbyFolderCoverArtwork{}, false
	}
}

func folderCoverArtworkForSeries(series *embySeriesGroup, imageType string) (EmbyFolderCoverArtwork, bool) {
	if series == nil {
		return EmbyFolderCoverArtwork{}, false
	}
	poster := strings.TrimSpace(series.PosterURL)
	backdrop := strings.TrimSpace(series.BackdropURL)
	switch folderCoverImageTypeValue(imageType) {
	case "Backdrop":
		if backdrop != "" {
			return EmbyFolderCoverArtwork{MediaID: series.ID, ImageType: "Backdrop", Tag: series.ID + "-bd", URL: backdrop}, true
		}
		return folderCoverArtworkForSeries(series, "Primary")
	case "Thumb":
		return folderCoverArtworkForSeries(series, "Primary")
	default:
		if poster != "" {
			return EmbyFolderCoverArtwork{MediaID: series.ID, ImageType: "Primary", Tag: series.ID, URL: poster}, true
		}
		return EmbyFolderCoverArtwork{}, false
	}
}

func folderCoverImageTypePreference(imageType string) []string {
	preferred := folderCoverImageTypeValue(imageType)
	values := []string{preferred, "Primary", "Backdrop", "Thumb"}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func folderCoverImageTypeValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "backdrop":
		return "Backdrop"
	case "thumb":
		return "Thumb"
	default:
		return "Primary"
	}
}
