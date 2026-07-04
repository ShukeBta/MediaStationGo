package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (o *OrganizerService) persistOrganizedSourceMetadata(ctx context.Context, plan organizeSourceFilePlan) {
	if o == nil || o.repo == nil || o.repo.Media == nil || plan.MetadataMatch == nil {
		return
	}
	libraryID := strings.TrimSpace(plan.TargetLibraryID)
	if libraryID == "" {
		return
	}
	media := organizedSourceMediaFromPlan(libraryID, plan)
	if info, err := os.Stat(plan.Target.Path); err == nil && !info.IsDir() {
		media.SizeBytes = info.Size()
	}
	if fileID, ok := fileIdentity(plan.Target.Path); ok {
		media.FileID = fileID
	}
	if err := o.repo.Media.Upsert(ctx, media); err != nil && o.log != nil {
		o.log.Warn("persist organized metadata failed",
			zap.String("path", plan.Target.Path),
			zap.String("library_id", libraryID),
			zap.Error(err))
	}
}

func organizedSourceMediaFromPlan(libraryID string, plan organizeSourceFilePlan) *model.Media {
	match := plan.MetadataMatch
	media := &model.Media{
		LibraryID:    libraryID,
		Title:        strings.TrimSpace(firstNonEmpty(match.Title, plan.Identity.ParsedTitle, plan.Identity.Title)),
		OriginalName: strings.TrimSpace(match.OriginalName),
		Overview:     match.Overview,
		PosterURL:    match.PosterURL,
		BackdropURL:  match.BackdropURL,
		Year:         firstPositiveInt(match.Year, plan.Identity.Year),
		Rating:       match.Rating,
		Path:         plan.Target.Path,
		Container:    strings.TrimPrefix(strings.ToLower(filepath.Ext(plan.Target.Path)), "."),
		SeasonNum:    plan.Identity.Season,
		EpisodeNum:   plan.Identity.Episode,
		TMDbID:       match.TMDbID,
		BangumiID:    match.BangumiID,
		DoubanID:     strings.TrimSpace(match.DoubanID),
		TheTVDBID:    strings.TrimSpace(match.TheTVDBID),
		Genres:       strings.Join(match.Genres, ","),
		Countries:    strings.Join(match.Countries, ","),
		Languages:    strings.Join(match.Languages, ","),
		NSFW:         match.NSFW,
		ScrapeStatus: "matched",
	}
	if normalizeOrganizeMediaType(plan.Layout.MediaType) == "movie" {
		media.SeasonNum = 0
		media.EpisodeNum = 0
	}
	return media
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
