package service

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type ManualScrapeRequest struct {
	ExpectedRevisions map[string]string `json:"expected_revisions,omitempty"`
	EpisodeEndNum     *int              `json:"episode_end_num,omitempty"`
	EpisodePartNum    *int              `json:"episode_part_num,omitempty"`
	SeasonNum         *int              `json:"season_num,omitempty"`
	EpisodeNum        *int              `json:"episode_num,omitempty"`
	Source            string            `json:"source"`
	MediaType         string            `json:"media_type"`
	Title             string            `json:"title"`
	OriginalName      string            `json:"original_name"`
	Overview          string            `json:"overview"`
	PosterURL         string            `json:"poster_url"`
	BackdropURL       string            `json:"backdrop_url"`
	Year              int               `json:"year"`
	ReleaseDate       string            `json:"release_date"`
	Rating            float32           `json:"rating"`
	TMDbID            int               `json:"tmdb_id"`
	BangumiID         int               `json:"bangumi_id"`
	DoubanID          string            `json:"douban_id"`
	TheTVDBID         string            `json:"thetvdb_id"`
	Languages         []string          `json:"languages"`
	Countries         []string          `json:"countries"`
	Genres            []string          `json:"genres"`
	Actors            []string          `json:"actors"`
	People            []PersonMetadata  `json:"people,omitempty"`
	NSFW              bool              `json:"nsfw"`
}

func (s *ScraperService) ApplyManualMatch(ctx context.Context, mediaID string, req ManualScrapeRequest) (*model.Media, error) {
	return s.ApplyManualMatchWithOptions(ctx, mediaID, req, ScrapeOptions{})
}

func (s *ScraperService) ApplyManualMatchWithOptions(ctx context.Context, mediaID string, req ManualScrapeRequest, options ScrapeOptions) (*model.Media, error) {
	if err := configureManualEpisodeMapping(req, &options); err != nil {
		return nil, err
	}
	options.automaticSelection = false
	if req.SeasonNum != nil || req.EpisodeNum != nil {
		if req.SeasonNum == nil || req.EpisodeNum == nil || *req.SeasonNum < 0 || *req.EpisodeNum <= 0 {
			return nil, errors.New("请同时指定有效的季号（>=0）和集号（>0）")
		}
		options.manualEpisodeIdentity = &episodeRef{Season: *req.SeasonNum, Episode: *req.EpisodeNum}
	}
	media, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil || media == nil {
		return nil, errors.New("media not found")
	}
	if err := checkScrapeRevision(req, *media); err != nil {
		return nil, err
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
	if req.SeasonNum != nil || req.EpisodeNum != nil || req.EpisodeEndNum != nil || req.EpisodePartNum != nil {
		return result, errors.New("指定季集号仅支持单集操作，请逐集处理冲突项")
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
	batchOptions.episodeFailures = make(map[[2]int]error)
	batchOptions.episodeValidation = make(map[[2]int]map[int]*TMDbEpisodeDetails)
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
		if err := checkScrapeRevision(req, *media); err != nil {
			result.Errors = append(result.Errors, ManualScrapeBatchError{MediaID: mediaID, Err: err})
			continue
		}
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
	// TMDB episode details were validated and saved with each successful row.
	for _, media := range appliedRows {
		if media.EpisodeNum > 0 {
			s.writeMediaNFOAfterScrape(ctx, media, libraryByID[media.LibraryID])
		}
	}
	s.invalidateMediaCache(ctx)
	return result, nil
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
	if req.TMDbID > 0 && (source == "" || source == "tmdb") {
		req.Genres = normalizeTMDbGenreValues(mediaType, req.Genres)
	}
	fallback := func() (*Match, error) {
		match := mergeManualRequestIntoMatch(&Match{}, req)
		if strings.TrimSpace(match.Title) == "" {
			return nil, errors.New("manual match title required")
		}
		return match, nil
	}
	switch {
	case req.TMDbID > 0 && (source == "" || source == "tmdb"):
		if mediaType == "tv" || mediaType == "anime" || mediaType == "variety" {
			if s.tmdb == nil || !s.tmdb.Enabled() {
				return nil, errors.New("TMDB 不可用，不能确认所选剧集")
			}
			match, err := s.tmdb.GetTVMatch(ctx, req.TMDbID)
			if err != nil {
				return nil, err
			}
			if match == nil {
				return nil, errors.New("TMDB 未返回所选剧集，未应用请求中的备用元数据")
			}
			return mergeManualRequestIntoMatch(match, req), nil
		}
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
