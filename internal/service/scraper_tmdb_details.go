package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

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
	if len(details.Actors) > 0 {
		updates["actors"] = strings.Join(details.Actors, ",")
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
	if err := s.persistPeople(ctx, details.People, details.Actors); err != nil {
		s.log.Warn("failed to save tmdb person metadata",
			zap.String("media_id", mediaID),
			zap.Int("tmdb_id", tmdbID),
			zap.Error(err))
	}
	s.log.Debug("enrich: saved extended metadata",
		zap.String("media_id", mediaID),
		zap.Strings("languages", details.Languages),
		zap.Strings("countries", details.Countries),
		zap.Strings("genres", details.Genres),
		zap.Strings("actors", details.Actors))
}

func (s *ScraperService) fetchAndSaveTMDbEpisodeDetails(ctx context.Context, m *model.Media, tmdbID int, matchYear int) bool {
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
	return s.saveTMDbEpisodeDetails(ctx, m, tmdbID, matchYear, episode)
}

func (s *ScraperService) saveTMDbEpisodeDetails(ctx context.Context, m *model.Media, tmdbID int, matchYear int, episode *TMDbEpisodeDetails) bool {
	saved, _ := s.saveTMDbEpisodeDetailsResult(ctx, m, tmdbID, matchYear, episode)
	return saved
}

func (s *ScraperService) saveTMDbEpisodeDetailsResult(ctx context.Context, m *model.Media, tmdbID int, matchYear int, episode *TMDbEpisodeDetails) (bool, error) {
	if m == nil || episode == nil {
		return false, nil
	}
	updates := tmdbEpisodeMetadataUpdates(m, episode, matchYear)
	if len(updates) == 0 {
		return false, nil
	}
	if err := s.repo.DB.Model(&model.Media{}).Where("id = ?", m.ID).
		Updates(updates).Error; err != nil {
		s.log.Warn("failed to save tmdb episode metadata",
			zap.String("media_id", m.ID),
			zap.Int("tmdb_id", tmdbID),
			zap.Int("season", m.SeasonNum),
			zap.Int("episode", m.EpisodeNum),
			zap.Error(err))
		return false, err
	}
	return true, nil
}

// applyTMDbEpisodeDetailsBatch fetches each season once and maps the result
// back to media rows by the persisted season/episode numbers. Strict mode is
// used by automatic ingestion: a missing episode title, mapping, or database
// write is an explicit failure instead of a false-success matched result.
func (s *ScraperService) applyTMDbEpisodeDetailsBatch(
	ctx context.Context,
	rows []*model.Media,
	tmdbID int,
	matchYear int,
	strict bool,
) (int, error) {
	if s == nil || s.tmdb == nil || !s.tmdb.Enabled() || tmdbID <= 0 {
		if strict {
			return 0, errors.New("tmdb episode details unavailable")
		}
		return 0, nil
	}
	bySeason := make(map[int][]*model.Media)
	seasons := make([]int, 0)
	for _, media := range rows {
		if media == nil || media.EpisodeNum <= 0 {
			continue
		}
		if _, ok := bySeason[media.SeasonNum]; !ok {
			seasons = append(seasons, media.SeasonNum)
		}
		bySeason[media.SeasonNum] = append(bySeason[media.SeasonNum], media)
	}
	if len(seasons) == 0 {
		if strict {
			return 0, errors.New("no episode rows available for tmdb detail mapping")
		}
		return 0, nil
	}
	sort.Ints(seasons)
	detailsBySeason := make(map[int]map[int]*TMDbEpisodeDetails, len(seasons))
	for _, season := range seasons {
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		default:
		}
		detailCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tmdbDetailsTimeout)
		episodes, err := s.tmdb.GetTVSeasonEpisodeDetails(detailCtx, tmdbID, season)
		cancel()
		if err != nil {
			if strict {
				return 0, fmt.Errorf("get tmdb season %d episode details: %w", season, err)
			}
			s.log.Debug("failed to get tmdb season details",
				zap.Int("tmdb_id", tmdbID),
				zap.Int("season", season),
				zap.Error(err))
			continue
		}
		detailsBySeason[season] = episodes
	}

	if strict {
		missing := make([]string, 0)
		for _, season := range seasons {
			episodes := detailsBySeason[season]
			for _, media := range bySeason[season] {
				episode := episodes[media.EpisodeNum]
				if episode == nil || strings.TrimSpace(episode.Name) == "" || len(tmdbEpisodeMetadataUpdates(media, episode, matchYear)) == 0 {
					missing = append(missing, fmt.Sprintf("S%02dE%02d", season, media.EpisodeNum))
				}
			}
		}
		if len(missing) > 0 {
			return 0, fmt.Errorf("tmdb episode details incomplete: %s", strings.Join(missing, ", "))
		}

		applied := 0
		err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, season := range seasons {
				episodes := detailsBySeason[season]
				for _, media := range bySeason[season] {
					updates := tmdbEpisodeMetadataUpdates(media, episodes[media.EpisodeNum], matchYear)
					res := tx.Model(&model.Media{}).Where("id = ?", media.ID).Updates(updates)
					if res.Error != nil {
						return fmt.Errorf("save tmdb episode S%02dE%02d: %w", season, media.EpisodeNum, res.Error)
					}
					if res.RowsAffected != 1 {
						return fmt.Errorf("save tmdb episode S%02dE%02d: updated %d rows", season, media.EpisodeNum, res.RowsAffected)
					}
					applied++
				}
			}
			return nil
		})
		if err != nil {
			return 0, err
		}
		return applied, nil
	}

	applied := 0
	for _, season := range seasons {
		episodes := detailsBySeason[season]
		for _, media := range bySeason[season] {
			episode := episodes[media.EpisodeNum]
			if episode == nil {
				continue
			}
			saved, err := s.saveTMDbEpisodeDetailsResult(ctx, media, tmdbID, matchYear, episode)
			if err == nil && saved {
				applied++
			}
		}
	}
	return applied, nil
}

func tmdbEpisodeMetadataUpdates(m *model.Media, episode *TMDbEpisodeDetails, matchYear int) map[string]any {
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
	if strings.TrimSpace(episode.StillURL) != "" {
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

func (s *ScraperService) enrichDeferredEpisodeDetails(ctx context.Context, rows []model.Media) error {
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
		if s.fetchAndSaveTMDbEpisodeDetails(ctx, media, media.TMDbID, media.Year) {
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
