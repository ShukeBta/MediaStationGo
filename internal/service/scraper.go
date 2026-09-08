// Package service — scraper orchestrator.
//
// ScraperService takes a Media row and tries to enrich it with metadata from
// local NFO first, then TMDb -> Douban -> Bangumi -> TheTVDB. Fanart.tv is
// artwork-only and upgrades poster/backdrop after a metadata match.
package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// EnrichOne runs the provider chain for a single media row.
func (s *ScraperService) EnrichOne(ctx context.Context, m *model.Media) error {
	return s.EnrichOneWithOptions(ctx, m, ScrapeOptions{})
}

func (s *ScraperService) EnrichOneWithOptions(ctx context.Context, m *model.Media, options ScrapeOptions) error {
	snapshot := *m
	options.sourceSnapshot = &snapshot
	options.automaticSelection = true
	lib, err := s.repo.Library.FindByID(ctx, m.LibraryID)
	if err != nil {
		return err
	}

	seriesLike := mediaIsEpisodic(m, lib)
	cloudMedia := isCloudMediaPath(m.Path) || (lib != nil && isCloudMediaPath(lib.Path))
	var local *LocalMetadata
	if !cloudMedia {
		if found, err := ReadLocalMetadata(m.Path, lib.Path, seriesLike); err == nil && found != nil {
			local = found
		} else if err != nil {
			s.log.Warn("read local metadata before scrape failed", zap.String("media_id", m.ID), zap.Error(err))
		}
	}
	if hinted, _ := pathHintMetadata(m.Path, seriesLike); hinted != nil {
		local = mergeScrapePathHintMetadata(local, hinted)
	}
	if local != nil {
		applyLocalMetadata(m, local)
	}

	year := mediaYearHint(m)

	if s.adult != nil && s.adult.Enabled() {
		if code := firstText(localAdultCode(local), AdultCodeFromMediaPath(m.Path), normalizeAdultCode(m.OriginalName), normalizeAdultCode(m.Title)); code != "" {
			if adultMatch, err := s.adult.Search(ctx, code); err == nil && adultMatch != nil {
				mergeLocalMetadataIntoMatch(adultMatch, local)
				return s.applyProviderMatchWithOptions(ctx, m, lib, adultMatch, options)
			} else if err != nil {
				s.log.Debug("adult metadata search failed", zap.String("media_id", m.ID), zap.String("code", code), zap.Error(err))
			}
		}
	}

	if match := s.matchFromMediaExternalIDs(ctx, m, lib); match != nil {
		s.applyFanartArtwork(ctx, match)
		mergeLocalMetadataIntoMatch(match, local)
		return s.applyProviderMatchWithOptions(ctx, m, lib, match, options)
	}

	candidates := scrapeQueryCandidatesWithRecognition(ctx, s.repo, m, lib)
	var query string
	match := (*Match)(nil)
	for _, candidate := range candidates {
		query = candidate
		var candidateMatch *Match
		if seriesLike && s.tmdb != nil && s.tmdb.Enabled() {
			candidateMatch, err = s.lookupTVStrict(ctx, m, candidate, year, options)
			if err != nil {
				return err
			}
		} else {
			candidateMatch = s.lookup(ctx, lib, m, candidate, year)
		}
		if candidateMatch == nil {
			continue
		}
		if candidateMatch.TMDbID > 0 && normalizeOrganizeMediaType(candidateMatch.MediaType) == "tv" && !episodePathTitleTrusted(m.Path, candidateMatch) {
			continue
		}
		if !organizeMetadataMatchTrusted(candidate, year, candidateMatch) {
			s.log.Warn("metadata scrape match rejected",
				zap.String("media_id", m.ID),
				zap.String("query", candidate),
				zap.String("title", candidateMatch.Title),
				zap.Int("source_year", year),
				zap.Int("match_year", candidateMatch.Year),
				zap.Int("tmdb_id", candidateMatch.TMDbID),
				zap.Int("bangumi_id", candidateMatch.BangumiID),
				zap.String("douban_id", candidateMatch.DoubanID),
				zap.String("thetvdb_id", candidateMatch.TheTVDBID))
			continue
		}
		preferLocalizedSearchTitle(candidate, candidateMatch)
		match = candidateMatch
		if match != nil {
			break
		}
	}
	if match == nil {
		if local != nil && !local.PathHint {
			return s.applyLocalMetadataMatch(ctx, m, local)
		}
		if err := s.repo.Media.UpdateWithCurrentSeriesKey(ctx, nil, m.ID, map[string]any{"scrape_status": "no_match"}); err != nil {
			return err
		}
		s.invalidateMediaCache(ctx)
		s.log.Info("metadata scrape no match",
			zap.String("media_id", m.ID),
			zap.String("query", query),
			zap.String("library_type", lib.Type))
		return nil
	}
	s.applyFanartArtwork(ctx, match)
	mergeLocalMetadataIntoMatch(match, local)

	return s.applyProviderMatchWithOptions(ctx, m, lib, match, options)
}

func (s *ScraperService) applyProviderMatch(ctx context.Context, m *model.Media, lib *model.Library, match *Match) error {
	return s.applyProviderMatchWithOptions(ctx, m, lib, match, ScrapeOptions{})
}

func (s *ScraperService) applyProviderMatchWithOptions(ctx context.Context, m *model.Media, lib *model.Library, match *Match, options ScrapeOptions) error {
	if m == nil {
		return fmt.Errorf("media is required")
	}
	snapshot := *m
	if options.sourceSnapshot != nil {
		snapshot = *options.sourceSnapshot
	}
	if options.episodeValidation == nil {
		options.episodeValidation = make(map[[2]int]map[int]*TMDbEpisodeDetails)
	}
	validated, err := s.validateEpisodeMatch(ctx, m, lib, match, options)
	if err != nil {
		return err
	}
	m = validated
	s.deriveAdultPosterIfNeeded(ctx, m, lib, match)
	mediaType := s.determineMediaTypeForMedia(lib, m, match)
	series, err := s.prepareScrapedSeries(ctx, m, lib, match, mediaType)
	if err != nil {
		return err
	}
	posterCandidate := match.PosterURL
	backdropCandidate := match.BackdropURL
	if series != nil && m != nil && m.EpisodeNum > 0 {
		// The provider match is show-level artwork. Episode details own the
		// playable row's backdrop and will fill it below.
		backdropCandidate = ""
	}
	posterURL, removePoster := s.prepareScrapedArtworkURL(ctx, m.ID, "poster_url", m.PosterURL, posterCandidate)
	backdropURL, removeBackdrop := s.prepareScrapedArtworkURL(ctx, m.ID, "backdrop_url", m.BackdropURL, backdropCandidate)
	updates := map[string]any{
		"title":         match.Title,
		"overview":      match.Overview,
		"poster_url":    posterURL,
		"backdrop_url":  backdropURL,
		"rating":        match.Rating,
		"year":          match.Year,
		"scrape_status": "matched",
	}
	if match.ReleaseDate != "" {
		updates["release_date"] = match.ReleaseDate
	}
	if match.OriginalName != "" {
		updates["original_name"] = match.OriginalName
	}
	if strings.TrimSpace(m.EpisodeTitle) != "" {
		updates["episode_title"] = strings.TrimSpace(m.EpisodeTitle)
	}
	if match.TMDbID > 0 {
		updates["tm_db_id"] = match.TMDbID
	}
	if match.BangumiID > 0 {
		updates["bangumi_id"] = match.BangumiID
	}
	if match.DoubanID != "" {
		updates["douban_id"] = match.DoubanID
	}
	if match.TheTVDBID != "" {
		updates["thetvdb_id"] = match.TheTVDBID
	}
	if match.NSFW {
		updates["nsfw"] = true
	}
	if len(match.Genres) > 0 {
		updates["genres"] = strings.Join(match.Genres, ",")
	}
	if len(match.Actors) > 0 {
		updates["actors"] = strings.Join(match.Actors, ",")
	}
	if len(match.Countries) > 0 {
		updates["countries"] = strings.Join(match.Countries, ",")
	}
	if len(match.Languages) > 0 {
		updates["languages"] = strings.Join(match.Languages, ",")
	}
	applyScrapeMediaTypeResets(updates, match)
	if match.TMDbID > 0 && mediaType == "tv" {
		updates["season_num"] = m.SeasonNum
		updates["episode_num"] = m.EpisodeNum
		updates["episode_end_num"] = m.EpisodeEndNum
		updates["episode_part_num"] = m.EpisodePartNum
		if episode := options.episodeValidation[[2]int{match.TMDbID, m.SeasonNum}][m.EpisodeNum]; episode != nil {
			for key, value := range tmdbEpisodeMetadataUpdates(m, episode, match.Year) {
				updates[key] = value
			}
		}
		if m.EpisodeEndNum > m.EpisodeNum {
			names := []string{}
			for number := m.EpisodeNum; number <= m.EpisodeEndNum; number++ {
				names = append(names, options.episodeValidation[[2]int{match.TMDbID, m.SeasonNum}][number].Name)
			}
			updates["episode_title"] = strings.Join(names, " / ")
			delete(updates, "duration_sec") // A single episode's runtime is not the file duration.
		}
		if m.EpisodePartNum > 0 {
			delete(updates, "duration_sec")
			updates["episode_title"] = fmt.Sprintf("%s（第 %d 段）", updates["episode_title"], m.EpisodePartNum)
		}
	}

	if err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current model.Media
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", m.ID).First(&current).Error; err != nil {
			return err
		}
		if !current.UpdatedAt.Equal(snapshot.UpdatedAt) || current.Path != snapshot.Path || current.LibraryID != snapshot.LibraryID || current.Title != snapshot.Title || current.OriginalName != snapshot.OriginalName || current.TMDbID != snapshot.TMDbID || current.SeasonNum != snapshot.SeasonNum || current.EpisodeNum != snapshot.EpisodeNum || current.ScrapeStatus != snapshot.ScrapeStatus || current.EpisodeTitle != snapshot.EpisodeTitle {
			return fmt.Errorf("媒体在刮削期间已变化，未覆盖，请重新校验")
		}
		if current.EpisodeEndNum != snapshot.EpisodeEndNum || current.EpisodePartNum != snapshot.EpisodePartNum {
			return fmt.Errorf("季集覆盖或分段映射已变化，未覆盖")
		}
		if err := savePreparedScrapedSeries(tx, series); err != nil {
			return err
		}
		return s.repo.Media.UpdateWithCurrentSeriesKey(ctx, tx, m.ID, updates)
	}); err != nil {
		return err
	}
	if match.TMDbID > 0 && mediaType == "tv" {
		s.log.Info("episode metadata validated and committed",
			zap.String("validation_version", "episode-path-v2"),
			zap.String("media_id", m.ID),
			zap.Bool("automatic_selection", options.automaticSelection),
			zap.Bool("explicit_episode_override", options.manualEpisodeIdentity != nil),
			zap.Int("old_tmdb_id", snapshot.TMDbID), zap.Int("tmdb_id", match.TMDbID),
			zap.Int("old_season", snapshot.SeasonNum), zap.Int("old_episode", snapshot.EpisodeNum),
			zap.Int("season", m.SeasonNum), zap.Int("episode", m.EpisodeNum))
	}
	if !options.deferPeople {
		if err := s.persistMatchPeople(ctx, match); err != nil {
			s.log.Warn("failed to save person metadata", zap.String("media_id", m.ID), zap.Error(err))
		}
	}
	if series != nil {
		s.removeCachedScrapedArtwork(series.removePoster, series.removeBackdrop)
	}
	s.removeCachedScrapedArtwork(removePoster, removeBackdrop)

	// Fetch extended metadata after the selected match is already saved.
	// Manual cloud/batch applies must not fail just because an optional provider
	// details request is slow or unavailable.
	if match.TMDbID > 0 && s.tmdb != nil && s.tmdb.Enabled() {
		if !options.deferTMDbDetails {
			s.fetchAndSaveTMDbExtendedMetadata(ctx, m.ID, match.TMDbID, mediaType)
		}
		if mediaType == "tv" && !options.DeferEpisodeDetails && options.episodeValidation[[2]int{match.TMDbID, m.SeasonNum}][m.EpisodeNum] == nil {
			s.fetchAndSaveTMDbEpisodeDetails(ctx, m, match.TMDbID, match.Year)
		}
	}
	if err := s.repo.Media.RefreshSearchAliases(ctx, m.ID); err != nil {
		return err
	}
	if !(options.DeferEpisodeDetails && m != nil && m.EpisodeNum > 0) {
		s.writeMediaNFOAfterScrape(ctx, m, lib)
	}
	if !options.deferCacheInvalidation {
		s.invalidateMediaCache(ctx)
	}
	s.hub.Publish("scrape", map[string]any{
		"media_id":   m.ID,
		"title":      match.Title,
		"tmdb_id":    match.TMDbID,
		"bangumi_id": match.BangumiID,
		"douban_id":  match.DoubanID,
		"thetvdb_id": match.TheTVDBID,
		"source":     map[bool]string{true: "adult"}[match.NSFW],
	})
	return nil
}

func applyScrapeMediaTypeResets(updates map[string]any, match *Match) {
	if updates == nil || match == nil {
		return
	}
	switch normalizeOrganizeMediaType(match.MediaType) {
	case "movie", "adult":
		updates["episode_end_num"] = 0
		updates["episode_part_num"] = 0
		updates["season_num"] = 0
		updates["episode_num"] = 0
		updates["episode_title"] = ""
		updates["series_id"] = ""
		if match.TMDbID <= 0 {
			updates["tm_db_id"] = 0
		}
		if match.BangumiID <= 0 {
			updates["bangumi_id"] = 0
		}
		if strings.TrimSpace(match.DoubanID) == "" {
			updates["douban_id"] = ""
		}
		if strings.TrimSpace(match.TheTVDBID) == "" {
			updates["thetvdb_id"] = ""
		}
	}
}
