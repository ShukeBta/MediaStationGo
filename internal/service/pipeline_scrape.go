package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

const (
	PipelineScrapeModeApply = "apply"
	PipelineScrapeModeSmart = "smart"
)

type PipelineScrapeService struct {
	repos   *repository.Container
	scraper *ScraperService
}

func NewPipelineScrapeService(repos *repository.Container, scraper *ScraperService) *PipelineScrapeService {
	return &PipelineScrapeService{repos: repos, scraper: scraper}
}

type PipelineScrapeRequest struct {
	Category  string   `json:"category,omitempty"`
	Title     string   `json:"title,omitempty"`
	Queries   []string `json:"queries,omitempty"`
	Provider  string   `json:"provider,omitempty"`
	MediaType string   `json:"media_type,omitempty"`
}

type PipelineScrapeResult struct {
	Mode         string               `json:"mode"`
	Query        string               `json:"query,omitempty"`
	MatchCount   int                  `json:"match_count,omitempty"`
	Match        *ExternalMediaResult `json:"match,omitempty"`
	MediaID      string               `json:"media_id"`
	MediaTitle   string               `json:"media_title,omitempty"`
	AppliedCount int                  `json:"applied_count,omitempty"`
	ScrapeStatus string               `json:"scrape_status,omitempty"`
}

func (s *PipelineScrapeService) Scrape(ctx context.Context, mediaID string, req PipelineScrapeRequest) (PipelineScrapeResult, error) {
	if s == nil || s.repos == nil || s.repos.Media == nil || s.scraper == nil {
		return PipelineScrapeResult{}, errors.New("pipeline scrape service unavailable")
	}
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return PipelineScrapeResult{}, errors.New("media id is required")
	}
	media, err := s.repos.Media.FindByID(ctx, mediaID)
	if err != nil || media == nil {
		return PipelineScrapeResult{}, errors.New("media not found")
	}
	queries := pipelineCompactStrings(req.Queries)
	if len(queries) == 0 && strings.TrimSpace(req.Title) != "" {
		queries = []string{strings.TrimSpace(req.Title)}
	}
	provider := strings.TrimSpace(req.Provider)
	mediaType := strings.TrimSpace(req.MediaType)
	options := pipelineScrapeOptions()

	if provider != "" && mediaType != "" {
		for _, query := range queries {
			matches, err := s.scraper.ManualSearch(ctx, media, query, provider, mediaType)
			if err != nil {
				return PipelineScrapeResult{}, err
			}
			matches = pipelineScrapeMatchesForMediaType(matches, mediaType)
			if len(matches) != 1 {
				continue
			}
			match := matches[0]
			refreshed, err := s.applySelectedMatch(ctx, media, match, options)
			if err != nil {
				return PipelineScrapeResult{}, err
			}
			result := PipelineScrapeResult{
				Mode:       PipelineScrapeModeApply,
				Query:      query,
				MatchCount: len(matches),
				Match:      &match,
				MediaID:    mediaID,
			}
			if refreshed != nil {
				result.MediaTitle = pipelineMediaDisplayTitle(*refreshed)
				result.ScrapeStatus = strings.TrimSpace(refreshed.ScrapeStatus)
				if result.ScrapeStatus == "matched" {
					propagated, err := s.propagateEpisodeMatch(ctx, media, refreshed, req)
					if err != nil {
						return PipelineScrapeResult{}, err
					}
					result.AppliedCount = 1 + propagated
				}
			}
			return result, nil
		}
	}

	if err := s.scraper.EnrichOneWithOptions(ctx, media, options); err != nil {
		return PipelineScrapeResult{}, err
	}
	refreshed, _ := s.repos.Media.FindByID(ctx, mediaID)
	result := PipelineScrapeResult{Mode: PipelineScrapeModeSmart, MediaID: mediaID}
	if refreshed != nil {
		result.MediaTitle = pipelineMediaDisplayTitle(*refreshed)
		result.ScrapeStatus = strings.TrimSpace(refreshed.ScrapeStatus)
		if result.ScrapeStatus == "matched" {
			propagated, err := s.propagateEpisodeMatch(ctx, media, refreshed, req)
			if err != nil {
				return PipelineScrapeResult{}, err
			}
			result.AppliedCount = 1 + propagated
		}
	}
	return result, nil
}

func pipelineScrapeMatchesForMediaType(matches []ExternalMediaResult, mediaType string) []ExternalMediaResult {
	mediaType = strings.TrimSpace(mediaType)
	if mediaType == "" {
		return matches
	}
	filtered := make([]ExternalMediaResult, 0, len(matches))
	for _, match := range matches {
		if strings.EqualFold(strings.TrimSpace(match.MediaType), mediaType) {
			filtered = append(filtered, match)
		}
	}
	return filtered
}

func (s *PipelineScrapeService) applySelectedMatch(
	ctx context.Context,
	media *model.Media,
	match ExternalMediaResult,
	options ScrapeOptions,
) (*model.Media, error) {
	lib, _ := s.repos.Library.FindByID(ctx, media.LibraryID)
	if err := s.scraper.applyProviderMatchWithOptions(ctx, media, lib, pipelineMatchFromExternalResult(match), options); err != nil {
		return nil, err
	}
	return s.repos.Media.FindByID(ctx, media.ID)
}

func pipelineMatchFromExternalResult(match ExternalMediaResult) *Match {
	return &Match{
		TMDbID:          match.TMDbID,
		BangumiID:       match.BangumiID,
		DoubanID:        match.DoubanID,
		TheTVDBID:       match.TheTVDBID,
		MediaType:       match.MediaType,
		Title:           match.Title,
		OriginalName:    match.OriginalName,
		Overview:        match.Overview,
		PosterURL:       match.PosterURL,
		BackdropURL:     match.BackdropURL,
		PreviewImages:   append([]string(nil), match.PreviewImages...),
		Year:            match.Year,
		ReleaseDate:     match.ReleaseDate,
		Rating:          match.Rating,
		DurationMinutes: match.DurationMinutes,
		Maker:           match.Maker,
		Languages:       append([]string(nil), match.Languages...),
		Countries:       append([]string(nil), match.Countries...),
		Genres:          append([]string(nil), match.Genres...),
		Actors:          append([]string(nil), match.Actors...),
		People:          append([]PersonMetadata(nil), match.People...),
		NSFW:            match.NSFW,
	}
}

func (s *PipelineScrapeService) propagateEpisodeMatch(ctx context.Context, media *model.Media, refreshed *model.Media, req PipelineScrapeRequest) (int, error) {
	category := normalizePipelineCategory(req.Category)
	if category != "tv" && category != "anime" {
		return 0, nil
	}
	if s == nil || s.repos == nil || s.repos.DB == nil || media == nil || refreshed == nil || refreshed.ScrapeStatus != "matched" {
		return 0, nil
	}
	folder := pipelineScrapeParentPath(media.Path)
	if folder == "" || folder == "/" {
		return 0, nil
	}
	if refreshed.TMDbID > 0 {
		// Keep the whole episode group visibly retryable until the season batch
		// has been validated and committed. A process interruption must not
		// leave a series-level match looking like complete episode metadata.
		if err := s.resetEpisodeGroupScrapeStatus(ctx, media, folder, "pending"); err != nil {
			return 0, fmt.Errorf("mark episode group pending: %w", err)
		}
		if s.scraper == nil {
			return 0, errors.New("pipeline tmdb episode detail service unavailable")
		}
	}
	updates := map[string]any{
		"title":         refreshed.Title,
		"overview":      refreshed.Overview,
		"poster_url":    refreshed.PosterURL,
		"rating":        refreshed.Rating,
		"year":          refreshed.Year,
		"scrape_status": refreshed.ScrapeStatus,
		"updated_at":    time.Now(),
	}
	// TMDb episode backdrops are per-episode stills. Do not copy the anchor's
	// backdrop to every sibling; the strict season batch below fills each row.
	if refreshed.TMDbID <= 0 {
		updates["backdrop_url"] = refreshed.BackdropURL
	} else {
		updates["scrape_status"] = "pending"
	}
	if refreshed.OriginalName != "" {
		updates["original_name"] = refreshed.OriginalName
	}
	if refreshed.ReleaseDate != "" {
		updates["release_date"] = refreshed.ReleaseDate
	}
	if refreshed.TMDbID > 0 {
		updates["tm_db_id"] = refreshed.TMDbID
	}
	if refreshed.BangumiID > 0 {
		updates["bangumi_id"] = refreshed.BangumiID
	}
	if refreshed.DoubanID != "" {
		updates["douban_id"] = refreshed.DoubanID
	}
	if refreshed.TheTVDBID != "" {
		updates["thetvdb_id"] = refreshed.TheTVDBID
	}
	if refreshed.Languages != "" {
		updates["languages"] = refreshed.Languages
	}
	if refreshed.Countries != "" {
		updates["countries"] = refreshed.Countries
	}
	if refreshed.Genres != "" {
		updates["genres"] = refreshed.Genres
	}
	if refreshed.Actors != "" {
		updates["actors"] = refreshed.Actors
	}
	if refreshed.NSFW {
		updates["nsfw"] = refreshed.NSFW
	}
	query := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND id <> ?", media.LibraryID, media.ID).
		Where("path LIKE ?", folder+"/%")
	if strings.TrimSpace(media.LibraryRootID) != "" {
		query = query.Where("library_root_id = ?", media.LibraryRootID)
	}
	res := query.Updates(updates)
	if res.Error != nil {
		return 0, res.Error
	}
	if refreshed.TMDbID > 0 {
		var rows []model.Media
		rowsQuery := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
			Where("library_id = ?", media.LibraryID).
			Where("path LIKE ?", folder+"/%")
		if strings.TrimSpace(media.LibraryRootID) != "" {
			rowsQuery = rowsQuery.Where("library_root_id = ?", media.LibraryRootID)
		}
		if err := rowsQuery.Order("season_num ASC, episode_num ASC, created_at ASC").Find(&rows).Error; err != nil {
			return 0, err
		}
		episodes := make([]*model.Media, 0, len(rows))
		for i := range rows {
			if rows[i].EpisodeNum > 0 {
				episodes = append(episodes, &rows[i])
			}
		}
		applied, err := s.scraper.applyTMDbEpisodeDetailsBatch(ctx, episodes, refreshed.TMDbID, refreshed.Year, true)
		if err != nil {
			return 0, err
		}
		if applied != len(episodes) {
			return 0, fmt.Errorf("tmdb episode details applied %d of %d rows", applied, len(episodes))
		}
		if strings.TrimSpace(refreshed.BackdropURL) != "" {
			clearQuery := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
				Where("library_id = ?", media.LibraryID).
				Where("path LIKE ?", folder+"/%").
				Where("backdrop_url = ?", refreshed.BackdropURL)
			if strings.TrimSpace(media.LibraryRootID) != "" {
				clearQuery = clearQuery.Where("library_root_id = ?", media.LibraryRootID)
			}
			if err := clearQuery.Update("backdrop_url", "").Error; err != nil {
				return 0, fmt.Errorf("clear shared series backdrop: %w", err)
			}
		}
		if err := s.resetEpisodeGroupScrapeStatus(ctx, media, folder, "matched"); err != nil {
			return 0, fmt.Errorf("mark episode group matched: %w", err)
		}
		lib, _ := s.repos.Library.FindByID(ctx, media.LibraryID)
		s.scraper.writeMediaNFOAfterScrape(ctx, media, lib)
		s.scraper.invalidateMediaCache(ctx)
	}
	return int(res.RowsAffected), nil
}

func (s *PipelineScrapeService) resetEpisodeGroupScrapeStatus(ctx context.Context, media *model.Media, folder string, status string) error {
	query := s.repos.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ?", media.LibraryID).
		Where("path LIKE ?", folder+"/%")
	if strings.TrimSpace(media.LibraryRootID) != "" {
		query = query.Where("library_root_id = ?", media.LibraryRootID)
	}
	return query.Update("scrape_status", status).Error
}

func pipelineScrapeParentPath(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	idx := strings.LastIndex(value, "/")
	if idx <= 0 {
		return ""
	}
	return value[:idx]
}

func pipelineScrapeOptions() ScrapeOptions {
	return ScrapeOptions{RetryNoMatch: true, IncludeMatched: true, DeferEpisodeDetails: true}
}
