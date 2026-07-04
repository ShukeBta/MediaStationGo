package service

import (
	"context"
	"errors"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type ManualScrapeRequest struct {
	Source         string   `json:"source"`
	MediaType      string   `json:"media_type"`
	Title          string   `json:"title"`
	OriginalName   string   `json:"original_name"`
	Overview       string   `json:"overview"`
	PosterURL      string   `json:"poster_url"`
	BackdropURL    string   `json:"backdrop_url"`
	Year           int      `json:"year"`
	ReleaseDate    string   `json:"release_date"`
	Rating         float32  `json:"rating"`
	TMDbID         int      `json:"tmdb_id"`
	BangumiID      int      `json:"bangumi_id"`
	DoubanID       string   `json:"douban_id"`
	TheTVDBID      string   `json:"thetvdb_id"`
	Languages      []string `json:"languages"`
	Countries      []string `json:"countries"`
	Genres         []string `json:"genres"`
	NSFW           bool     `json:"nsfw"`
	EpisodeArtwork *bool    `json:"episode_artwork,omitempty"`
	EpisodeImages  *bool    `json:"episode_images,omitempty"`
}

func (r ManualScrapeRequest) EpisodeArtworkOption() *bool {
	if r.EpisodeImages != nil {
		return r.EpisodeImages
	}
	return r.EpisodeArtwork
}

func (s *ScraperService) ApplyManualMatch(ctx context.Context, mediaID string, req ManualScrapeRequest) (*model.Media, error) {
	return s.ApplyManualMatchWithOptions(ctx, mediaID, req, ScrapeOptions{EpisodeArtwork: req.EpisodeArtworkOption()})
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
	case source == "adult":
		if match := s.manualAdultMatch(ctx, firstText(req.OriginalName, req.Title)); match != nil {
			return mergeManualRequestIntoMatch(match, req), nil
		}
	}
	return fallback()
}
