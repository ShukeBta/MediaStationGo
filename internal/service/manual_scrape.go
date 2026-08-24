package service

import (
	"context"
	"errors"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type ManualScrapeRequest struct {
	Source       string           `json:"source"`
	MediaType    string           `json:"media_type"`
	Title        string           `json:"title"`
	OriginalName string           `json:"original_name"`
	Overview     string           `json:"overview"`
	PosterURL    string           `json:"poster_url"`
	BackdropURL  string           `json:"backdrop_url"`
	Year         int              `json:"year"`
	ReleaseDate  string           `json:"release_date"`
	Rating       float32          `json:"rating"`
	TMDbID       int              `json:"tmdb_id"`
	BangumiID    int              `json:"bangumi_id"`
	DoubanID     string           `json:"douban_id"`
	TheTVDBID    string           `json:"thetvdb_id"`
	Languages    []string         `json:"languages"`
	Countries    []string         `json:"countries"`
	Genres       []string         `json:"genres"`
	Actors       []string         `json:"actors"`
	People       []PersonMetadata `json:"people,omitempty"`
	NSFW         bool             `json:"nsfw"`
}

func (s *ScraperService) ApplyManualMatch(ctx context.Context, mediaID string, req ManualScrapeRequest) (*model.Media, error) {
	return s.ApplyManualMatchWithOptions(ctx, mediaID, req, ScrapeOptions{})
}

func (s *ScraperService) ApplyManualMatchWithOptions(ctx context.Context, mediaID string, req ManualScrapeRequest, options ScrapeOptions) (*model.Media, error) {
	media, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil || media == nil {
		return nil, errors.New("media not found")
	}
	lib, _ := s.repo.Library.FindByID(ctx, media.LibraryID)
	match, err := s.manualRequestMatch(ctx, req)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(match.Title) == "" {
		return nil, errors.New("manual match title required")
	}
	if err := s.applyProviderMatchWithOptions(ctx, media, lib, match, options); err != nil {
		return nil, err
	}
	return s.repo.Media.FindByID(ctx, mediaID)
}

type ManualScrapeBatchError struct {
	MediaID string
	Err     error
}

type ManualScrapeBatchResult struct {
	AppliedIDs []string
	Errors     []ManualScrapeBatchError
}

func (s *ScraperService) ApplyManualMatchBatchWithOptions(ctx context.Context, mediaIDs []string, req ManualScrapeRequest, options ScrapeOptions) (ManualScrapeBatchResult, error) {
	result := ManualScrapeBatchResult{
		AppliedIDs: make([]string, 0, len(mediaIDs)),
		Errors:     make([]ManualScrapeBatchError, 0),
	}
	match, err := s.manualRequestMatch(ctx, req)
	if err != nil {
		return result, err
	}
	if strings.TrimSpace(match.Title) == "" {
		return result, errors.New("manual match title required")
	}

	rows, err := s.repo.Media.FindByIDs(ctx, mediaIDs)
	if err != nil {
		return result, err
	}
	mediaByID := make(map[string]*model.Media, len(rows))
	libraryIDs := make([]string, 0, len(rows))
	seenLibraryIDs := make(map[string]struct{})
	for i := range rows {
		mediaByID[rows[i].ID] = &rows[i]
		if _, exists := seenLibraryIDs[rows[i].LibraryID]; !exists {
			seenLibraryIDs[rows[i].LibraryID] = struct{}{}
			libraryIDs = append(libraryIDs, rows[i].LibraryID)
		}
	}
	var libraries []model.Library
	if len(libraryIDs) > 0 {
		if err := s.repo.DB.WithContext(ctx).Where("id IN ?", libraryIDs).Find(&libraries).Error; err != nil {
			return result, err
		}
	}
	libraryByID := make(map[string]*model.Library, len(libraries))
	for i := range libraries {
		libraryByID[libraries[i].ID] = &libraries[i]
	}

	batchOptions := options
	batchOptions.DeferEpisodeDetails = true
	batchOptions.deferTMDbDetails = true
	batchOptions.deferPeople = true
	batchOptions.deferCacheInvalidation = true
	appliedRows := make([]*model.Media, 0, len(rows))
	for _, mediaID := range mediaIDs {
		media := mediaByID[mediaID]
		if media == nil {
			result.Errors = append(result.Errors, ManualScrapeBatchError{MediaID: mediaID, Err: errors.New("media not found")})
			continue
		}
		mediaMatch := cloneManualScrapeMatch(match)
		if err := s.applyProviderMatchWithOptions(ctx, media, libraryByID[media.LibraryID], mediaMatch, batchOptions); err != nil {
			result.Errors = append(result.Errors, ManualScrapeBatchError{MediaID: mediaID, Err: err})
			continue
		}
		result.AppliedIDs = append(result.AppliedIDs, mediaID)
		appliedRows = append(appliedRows, media)
	}

	if len(appliedRows) == 0 {
		return result, nil
	}
	if err := s.persistMatchPeople(ctx, match); err != nil {
		s.log.Warn("failed to save batch person metadata", zap.Int("media_count", len(appliedRows)), zap.Error(err))
	}
	s.applyManualBatchTMDbEpisodeDetails(ctx, appliedRows, libraryByID, match)
	for _, media := range appliedRows {
		if media.EpisodeNum > 0 {
			s.writeMediaNFOAfterScrape(ctx, media, libraryByID[media.LibraryID])
		}
	}
	s.invalidateMediaCache(ctx)
	return result, nil
}

type manualScrapeSeasonKey struct {
	TMDbID int
	Season int
}

func (s *ScraperService) applyManualBatchTMDbEpisodeDetails(ctx context.Context, rows []*model.Media, libraries map[string]*model.Library, match *Match) {
	if s == nil || s.tmdb == nil || !s.tmdb.Enabled() || match == nil || match.TMDbID <= 0 {
		return
	}
	groups := make(map[manualScrapeSeasonKey][]*model.Media)
	keys := make([]manualScrapeSeasonKey, 0)
	for _, media := range rows {
		if media == nil || media.EpisodeNum <= 0 || s.determineMediaTypeForMedia(libraries[media.LibraryID], media, match) != "tv" {
			continue
		}
		key := manualScrapeSeasonKey{TMDbID: match.TMDbID, Season: media.SeasonNum}
		if _, exists := groups[key]; !exists {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], media)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].TMDbID != keys[j].TMDbID {
			return keys[i].TMDbID < keys[j].TMDbID
		}
		return keys[i].Season < keys[j].Season
	})
	for _, key := range keys {
		select {
		case <-ctx.Done():
			return
		default:
		}
		detailCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tmdbDetailsTimeout)
		episodes, err := s.tmdb.GetTVSeasonEpisodeDetails(detailCtx, key.TMDbID, key.Season)
		cancel()
		if err != nil {
			s.log.Debug("failed to get tmdb season details",
				zap.Int("tmdb_id", key.TMDbID),
				zap.Int("season", key.Season),
				zap.Error(err))
			continue
		}
		for _, media := range groups[key] {
			episode := episodes[media.EpisodeNum]
			if episode == nil {
				continue
			}
			s.saveTMDbEpisodeDetails(ctx, media, key.TMDbID, match.Year, episode)
		}
	}
}

func cloneManualScrapeMatch(match *Match) *Match {
	if match == nil {
		return nil
	}
	cloned := *match
	cloned.PreviewImages = append([]string(nil), match.PreviewImages...)
	cloned.Languages = append([]string(nil), match.Languages...)
	cloned.Countries = append([]string(nil), match.Countries...)
	cloned.Genres = append([]string(nil), match.Genres...)
	cloned.Actors = append([]string(nil), match.Actors...)
	cloned.People = append([]PersonMetadata(nil), match.People...)
	return &cloned
}

func (s *ScraperService) manualRequestMatch(ctx context.Context, req ManualScrapeRequest) (*Match, error) {
	source := strings.ToLower(strings.TrimSpace(req.Source))
	mediaType := normalizeMediaType(req.MediaType, req.Title, "")
	fallback := func() (*Match, error) {
		match := mergeManualRequestIntoMatch(&Match{}, req)
		if strings.TrimSpace(match.Title) == "" {
			return nil, errors.New("manual match title required")
		}
		return match, nil
	}
	switch {
	case req.TMDbID > 0 && (source == "" || source == "tmdb"):
		if match := s.manualTMDbMatchByID(ctx, req.TMDbID, mediaType); match != nil {
			return mergeManualRequestIntoMatch(match, req), nil
		}
	case req.BangumiID > 0 && (source == "" || source == "bangumi"):
		if s.bangumi != nil {
			match, err := s.bangumi.GetSubject(ctx, req.BangumiID)
			if err == nil && match != nil {
				return mergeManualRequestIntoMatch(match, req), nil
			}
		}
	case strings.TrimSpace(req.TheTVDBID) != "" && (source == "" || source == "thetvdb"):
		if s.thetvdb != nil {
			match, err := s.thetvdb.GetSeriesMatchByID(ctx, req.TheTVDBID)
			if err == nil && match != nil {
				return mergeManualRequestIntoMatch(match, req), nil
			}
		}
	case strings.TrimSpace(req.DoubanID) != "" && (source == "" || source == "douban"):
		if s.douban != nil {
			match, err := s.douban.GetMatchByID(ctx, req.DoubanID)
			if err == nil && match != nil {
				return mergeManualRequestIntoMatch(match, req), nil
			}
		}
	}
	return fallback()
}
