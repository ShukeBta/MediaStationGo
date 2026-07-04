package service

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *ScraperService) fetchAndSaveTMDbExtendedMetadata(ctx context.Context, mediaID string, tmdbID int, mediaType string) {
	detailCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tmdbDetailsTimeout)
	details, err := s.tmdb.GetDetails(detailCtx, tmdbID, mediaType)
	cancel()
	if err != nil {
		s.log.Warn("failed to get details from tmdb",
			zap.Int("tmdb_id", tmdbID),
			zap.String("type", mediaType),
			zap.Error(err))
		return
	}
	if details == nil {
		return
	}
	updates := map[string]any{}
	if len(details.Languages) > 0 {
		updates["languages"] = strings.Join(details.Languages, ",")
	}
	if len(details.Countries) > 0 {
		updates["countries"] = strings.Join(details.Countries, ",")
	}
	if len(details.Genres) > 0 {
		updates["genres"] = strings.Join(details.Genres, ",")
	}
	if len(updates) > 0 {
		if err := s.repo.DB.Model(&model.Media{}).Where("id = ?", mediaID).
			Updates(updates).Error; err != nil {
			s.log.Warn("failed to save tmdb extended metadata",
				zap.String("media_id", mediaID),
				zap.Int("tmdb_id", tmdbID),
				zap.Error(err))
		}
	}
	s.log.Debug("enrich: saved extended metadata",
		zap.String("media_id", mediaID),
		zap.Strings("languages", details.Languages),
		zap.Strings("countries", details.Countries),
		zap.Strings("genres", details.Genres))
}

func (s *ScraperService) fetchAndSaveTMDbEpisodeDetails(ctx context.Context, m *model.Media, tmdbID int, matchYear int, options ScrapeOptions) bool {
	if s == nil || s.tmdb == nil || !s.tmdb.Enabled() || m == nil || tmdbID <= 0 || m.EpisodeNum <= 0 {
		return false
	}
	episodeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tmdbDetailsTimeout)
	episode, err := s.tmdb.GetTVEpisodeDetails(episodeCtx, tmdbID, m.SeasonNum, m.EpisodeNum)
	cancel()
	if err != nil {
		s.log.Debug("failed to get tmdb episode details",
			zap.String("media_id", m.ID),
			zap.Int("tmdb_id", tmdbID),
			zap.Int("season", m.SeasonNum),
			zap.Int("episode", m.EpisodeNum),
			zap.Error(err))
		return false
	}
	if episode == nil {
		return false
	}
	updates := tmdbEpisodeMetadataUpdates(m, episode, matchYear, options)
	if len(updates) == 0 {
		return false
	}
	if err := s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).
		Updates(updates).Error; err != nil {
		s.log.Warn("failed to save tmdb episode metadata",
			zap.String("media_id", m.ID),
			zap.Int("tmdb_id", tmdbID),
			zap.Int("season", m.SeasonNum),
			zap.Int("episode", m.EpisodeNum),
			zap.Error(err))
		return false
	}
	return true
}

func tmdbEpisodeMetadataUpdates(m *model.Media, episode *TMDbEpisodeDetails, matchYear int, options ScrapeOptions) map[string]any {
	updates := map[string]any{}
	if episode == nil {
		return updates
	}
	// Keep original_name at series level. Per-episode names can split one show
	// into multiple cards because original_name participates in grouping.
	if strings.TrimSpace(episode.Name) != "" {
		updates["episode_title"] = strings.TrimSpace(episode.Name)
	}
	if strings.TrimSpace(episode.Overview) != "" {
		updates["overview"] = strings.TrimSpace(episode.Overview)
	}
	if strings.TrimSpace(episode.StillURL) != "" && options.episodeArtworkEnabled() {
		updates["backdrop_url"] = strings.TrimSpace(episode.StillURL)
	}
	if episode.Rating > 0 {
		updates["rating"] = episode.Rating
	}
	if episode.AirYear > 0 && matchYear <= 0 {
		updates["year"] = episode.AirYear
	}
	if m != nil && episode.Runtime > 0 && m.DurationSec <= 0 {
		updates["duration_sec"] = episode.Runtime * 60
	}
	return updates
}

func (s *ScraperService) enrichDeferredEpisodeDetails(ctx context.Context, rows []model.Media, options ScrapeOptions) error {
	if s == nil || s.tmdb == nil || !s.tmdb.Enabled() {
		return nil
	}
	for i := range rows {
		if rows[i].EpisodeNum <= 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		media, err := s.repo.Media.FindByID(ctx, rows[i].ID)
		if err != nil || media == nil {
			s.log.Debug("deferred episode metadata media missing", zap.String("media_id", rows[i].ID), zap.Error(err))
			continue
		}
		if media.TMDbID <= 0 || media.EpisodeNum <= 0 {
			continue
		}
		lib, _ := s.repo.Library.FindByID(ctx, media.LibraryID)
		if !mediaIsEpisodic(media, lib) {
			continue
		}
		if s.fetchAndSaveTMDbEpisodeDetails(ctx, media, media.TMDbID, media.Year, options) {
			s.writeMediaNFOAfterScrape(ctx, media, lib)
			s.invalidateMediaCache(ctx)
		}
		if i < len(rows)-1 {
			if delay := s.scrapeDelay(ctx); delay > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
			}
		}
	}
	return nil
}

func (s *ScraperService) writeMediaNFOAfterScrape(ctx context.Context, m *model.Media, lib *model.Library) {
	if s == nil || m == nil {
		return
	}
	cloudMedia := isCloudMediaPath(m.Path) || (lib != nil && isCloudMediaPath(lib.Path))
	if cloudMedia {
		return
	}
	refreshed, err := s.repo.Media.FindByID(ctx, m.ID)
	if err != nil || refreshed == nil {
		return
	}
	if path, err := WriteMediaNFO(refreshed); err != nil {
		s.log.Warn("write nfo after scrape failed", zap.String("media_id", m.ID), zap.Error(err))
	} else {
		s.log.Debug("write nfo after scrape", zap.String("media_id", m.ID), zap.String("path", path))
	}
}
