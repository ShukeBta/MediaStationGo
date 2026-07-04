package service

import (
	"context"
	"errors"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type MediaMetadataUpdate struct {
	Title        *string  `json:"title"`
	OriginalName *string  `json:"original_name"`
	Overview     *string  `json:"overview"`
	PosterURL    *string  `json:"poster_url"`
	BackdropURL  *string  `json:"backdrop_url"`
	Year         *int     `json:"year"`
	ReleaseDate  *string  `json:"release_date"`
	Rating       *float32 `json:"rating"`
	SeasonNum    *int     `json:"season_num"`
	EpisodeNum   *int     `json:"episode_num"`
	TMDbID       *int     `json:"tmdb_id"`
	BangumiID    *int     `json:"bangumi_id"`
	DoubanID     *string  `json:"douban_id"`
	TheTVDBID    *string  `json:"thetvdb_id"`
	Languages    *string  `json:"languages"`
	Countries    *string  `json:"countries"`
	Genres       *string  `json:"genres"`
	NSFW         *bool    `json:"nsfw"`
}

func (s *MediaService) UpdateMetadata(ctx context.Context, id string, req MediaMetadataUpdate) (*model.Media, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil {
		return nil, errors.New("media service unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("media id required")
	}
	if existing, err := s.repo.Media.FindByID(ctx, id); err != nil {
		return nil, err
	} else if existing == nil {
		return nil, errors.New("media not found")
	}
	updates := map[string]any{"scrape_status": "matched"}
	if req.Title != nil {
		title := strings.TrimSpace(*req.Title)
		if title == "" {
			return nil, errors.New("title required")
		}
		updates["title"] = title
	}
	if req.OriginalName != nil {
		updates["original_name"] = strings.TrimSpace(*req.OriginalName)
	}
	if req.Overview != nil {
		updates["overview"] = strings.TrimSpace(*req.Overview)
	}
	if req.PosterURL != nil {
		updates["poster_url"] = strings.TrimSpace(*req.PosterURL)
	}
	if req.BackdropURL != nil {
		updates["backdrop_url"] = strings.TrimSpace(*req.BackdropURL)
	}
	if req.Year != nil {
		updates["year"] = clampNonNegativeInt(*req.Year)
	}
	if req.ReleaseDate != nil {
		updates["release_date"] = normalizeReleaseDate(*req.ReleaseDate)
	}
	if req.Rating != nil {
		updates["rating"] = clampRating(*req.Rating)
	}
	if req.SeasonNum != nil {
		updates["season_num"] = clampNonNegativeInt(*req.SeasonNum)
	}
	if req.EpisodeNum != nil {
		updates["episode_num"] = clampNonNegativeInt(*req.EpisodeNum)
	}
	if req.TMDbID != nil {
		updates["tm_db_id"] = clampNonNegativeInt(*req.TMDbID)
	}
	if req.BangumiID != nil {
		updates["bangumi_id"] = clampNonNegativeInt(*req.BangumiID)
	}
	if req.DoubanID != nil {
		updates["douban_id"] = strings.TrimSpace(*req.DoubanID)
	}
	if req.TheTVDBID != nil {
		updates["thetvdb_id"] = strings.TrimSpace(*req.TheTVDBID)
	}
	if req.Languages != nil {
		updates["languages"] = normalizeMetadataCSV(*req.Languages)
	}
	if req.Countries != nil {
		updates["countries"] = normalizeMetadataCSV(*req.Countries)
	}
	if req.Genres != nil {
		updates["genres"] = normalizeMetadataCSV(*req.Genres)
	}
	if req.NSFW != nil {
		updates["nsfw"] = *req.NSFW
	}
	if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.invalidateMediaCache(ctx)
	return s.repo.Media.FindByID(ctx, id)
}

func normalizeMetadataCSV(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case ',', '，', ';', '；', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key := strings.ToLower(part)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, part)
	}
	return strings.Join(out, ",")
}

func clampNonNegativeInt(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

func clampRating(value float32) float32 {
	if value < 0 {
		return 0
	}
	if value > 10 {
		return 10
	}
	return value
}
