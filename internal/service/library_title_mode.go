package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const (
	LibraryTitleModeSmart    = "smart"
	LibraryTitleModeFilename = "filename"
)

func NormalizeLibraryTitleMode(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", LibraryTitleModeSmart:
		return LibraryTitleModeSmart, nil
	case LibraryTitleModeFilename:
		return LibraryTitleModeFilename, nil
	default:
		return "", errors.New("title_mode must be smart or filename")
	}
}

func libraryPreservesSourceTitle(lib *model.Library) bool {
	if lib == nil {
		return false
	}
	mode, err := NormalizeLibraryTitleMode(lib.TitleMode)
	return err == nil && mode == LibraryTitleModeFilename
}

func (s *ScannerService) libraryAllowsAutoScrape(ctx context.Context, libraryID string) bool {
	if s == nil || s.repo == nil || s.repo.Library == nil {
		return false
	}
	lib, err := s.repo.Library.FindByID(ctx, strings.TrimSpace(libraryID))
	return err == nil && lib != nil && !libraryPreservesSourceTitle(lib)
}

func sourceFilenameTitle(raw string) string {
	base := pathBaseSlash(raw)
	if base == "" {
		base = strings.TrimSpace(raw)
	}
	return strings.TrimSpace(strings.TrimSuffix(base, filepath.Ext(base)))
}

func preserveSourceTitleIdentity(media *model.Media, raw string) {
	if media == nil {
		return
	}
	if title := sourceFilenameTitle(raw); title != "" {
		media.Title = title
	}
	media.Year = 0
	media.SeasonNum = 0
	media.EpisodeNum = 0
	media.SeriesID = ""
	media.EpisodeTitle = ""
	media.PreserveSourceTitle = true
}
