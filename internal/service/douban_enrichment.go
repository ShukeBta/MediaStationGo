package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

var ErrDoubanBindingChanged = errors.New("douban binding changed during enrichment")

// EnrichFromDouban refreshes provider evidence, filling only absent editable fields.
// Existing titles, ratings and images remain authoritative after manual editing.
func (s *ScraperService) EnrichFromDouban(ctx context.Context, mediaID string) (*model.Media, error) {
	if s == nil || s.repo == nil || s.douban == nil {
		return nil, errors.New("douban enrichment unavailable")
	}
	item, err := s.repo.Media.FindByID(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, gorm.ErrRecordNotFound
	}
	id := strings.TrimSpace(item.DoubanID)
	if id == "" {
		return nil, errors.New("douban id required")
	}
	if item.NSFW {
		return nil, errors.New("douban enrichment requires a movie or series")
	}
	kind := "movie"
	if item.EpisodeNum > 0 || item.SeriesID != "" {
		kind = "series"
	}
	details, degraded, err := s.douban.GetEnrichmentMatchByID(ctx, id, kind)
	if err != nil {
		return nil, err
	}
	if details == nil {
		return nil, ErrDoubanSubjectNotFound
	}
	if err := s.saveDoubanEnrichment(ctx, mediaID, id, details, degraded); err != nil {
		return nil, err
	}
	s.invalidateMediaCache(ctx)
	enriched, err := s.repo.Media.FindByID(ctx, mediaID)
	if err == nil && enriched != nil && s.images != nil {
		_ = s.images.RemoveFailed(enriched.PosterURL)
	}
	return enriched, err
}

func (s *ScraperService) saveDoubanEnrichment(ctx context.Context, mediaID, doubanID string, details *Match, degraded bool) error {
	err := s.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var live model.Media
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&live, "id = ?", mediaID).Error; err != nil {
			return err
		}
		if live.DoubanID != doubanID {
			return ErrDoubanBindingChanged
		}
		now := time.Now().UTC()
		updates := missingDoubanMetadata(live, details)
		updates["douban_rating"] = clampRating(details.Rating)
		updates["douban_degraded"] = degraded
		updates["douban_fetched_at"] = now
		if err := s.repo.Media.UpdateWithCurrentSeriesKey(ctx, tx, mediaID, updates); err != nil {
			return err
		}
		snapshot := model.DoubanSnapshot{MediaID: mediaID, DoubanID: doubanID, Payload: string(details.RawJSON), Degraded: degraded, FetchedAt: now}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "media_id"}}, UpdateAll: true}).Create(&snapshot).Error
	})
	if err != nil {
		return err
	}
	return s.repo.Media.RefreshSearchAliases(ctx, mediaID)
}

func missingDoubanMetadata(live model.Media, details *Match) map[string]any {
	updates := map[string]any{}
	set := func(column, current, value string) {
		if strings.TrimSpace(current) == "" && strings.TrimSpace(value) != "" {
			updates[column] = strings.TrimSpace(value)
		}
	}
	set("title", live.Title, details.Title)
	set("original_name", live.OriginalName, details.OriginalName)
	set("overview", live.Overview, details.Overview)
	set("poster_url", live.PosterURL, details.PosterURL)
	if original, err := url.Parse(live.PosterURL); err == nil && isDoubanImageHost(original.Host) {
		canonical := (&DoubanProvider{}).ResolveArtworkURL(context.Background(), live.PosterURL)
		if canonical != "" && canonical != live.PosterURL {
			updates["poster_url"] = canonical
		}
	}
	set("release_date", live.ReleaseDate, details.ReleaseDate)
	set("languages", live.Languages, strings.Join(details.Languages, ","))
	set("countries", live.Countries, strings.Join(details.Countries, ","))
	set("genres", live.Genres, strings.Join(details.Genres, ","))
	set("actors", live.Actors, strings.Join(details.Actors, ","))
	if live.Year == 0 && details.Year > 0 {
		updates["year"] = details.Year
	}
	if live.Rating == 0 && details.Rating > 0 {
		updates["rating"] = clampRating(details.Rating)
	}
	return updates
}

// runDoubanEnrichment reads 100 stale subjects per batch and backs off on
// temporary upstream failures. Successful subjects remain fresh for 24 hours.
func (s *ScraperService) runDoubanEnrichment(ctx context.Context) error {
	if s == nil || s.repo == nil || s.douban == nil {
		return nil
	}
	ctx = withDoubanTaskRouteReset(ctx)
	for {
		var candidates []model.Media
		if err := s.repo.DB.WithContext(ctx).Where("douban_id <> ? AND nsfw = ?", "", false).
			Where("douban_fetched_at IS NULL OR douban_fetched_at < ?", time.Now().UTC().Add(-24*time.Hour)).
			Order("douban_fetched_at asc, id asc").Limit(100).Find(&candidates).Error; err != nil {
			return err
		}
		for i, candidate := range candidates {
			if _, err := s.EnrichFromDouban(ctx, candidate.ID); err != nil {
				if !errors.Is(err, ErrDoubanSubjectNotFound) && !errors.Is(err, ErrDoubanBindingChanged) {
					return fmt.Errorf("douban enrichment: %w", err)
				}
				// Missing subjects receive a retry date to avoid starving later entries.
				if errors.Is(err, ErrDoubanSubjectNotFound) {
					if err := s.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("id = ? AND douban_id = ?", candidate.ID, candidate.DoubanID).Update("douban_fetched_at", time.Now().UTC()).Error; err != nil {
						return err
					}
				}
			}
			if i+1 < len(candidates) {
				timer := time.NewTimer(2 * time.Second)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
		}
		if len(candidates) < 100 {
			return nil
		}
	}
}
