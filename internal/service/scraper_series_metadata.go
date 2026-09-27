package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type preparedScrapedSeries struct {
	row            model.Series
	removePoster   string
	removeBackdrop string
}

// prepareScrapedSeries separates show-level metadata from the playable
// episode row before TMDb episode details replace Media.BackdropURL with the
// episode still. Media.SeriesID is deliberately left unchanged so introducing
// persisted metadata does not change existing Emby public item IDs.
func (s *ScraperService) prepareScrapedSeries(
	ctx context.Context,
	media *model.Media,
	library *model.Library,
	match *Match,
	mediaType string,
) (*preparedScrapedSeries, error) {
	if s == nil || s.repo == nil || s.repo.DB == nil || media == nil || match == nil || mediaType != "tv" || match.TMDbID <= 0 {
		return nil, nil
	}
	libraryID := strings.TrimSpace(media.LibraryID)
	title := strings.TrimSpace(match.Title)
	if libraryID == "" || title == "" || !mediaIsEpisodic(media, library) {
		return nil, nil
	}

	series, err := s.findScrapedSeries(ctx, media, match)
	if err != nil {
		return nil, err
	}
	if series == nil {
		series = &model.Series{
			Base: model.Base{ID: scrapedSeriesMetadataID(libraryID, match)},
		}
	}
	series.LibraryID = libraryID
	series.Title = title
	posterURL, removePoster := s.prepareScrapedArtworkURL(
		ctx, series.ID, "series.poster_url", series.PosterURL, match.PosterURL,
	)
	backdropURL, removeBackdrop := s.prepareScrapedArtworkURL(
		ctx, series.ID, "series.backdrop_url", series.BackdropURL, match.BackdropURL,
	)
	series.PosterURL = posterURL
	series.BackdropURL = backdropURL
	if value := strings.TrimSpace(match.Overview); value != "" {
		series.Overview = value
	}
	if match.Rating > 0 {
		series.Rating = match.Rating
	}
	if match.Year > 0 {
		series.Year = match.Year
	}
	if match.TMDbID > 0 {
		series.TMDbID = match.TMDbID
	}
	if match.BangumiID > 0 {
		series.BangumiID = match.BangumiID
	}
	if value := strings.TrimSpace(match.DoubanID); value != "" {
		series.DoubanID = value
	}
	if value := strings.TrimSpace(match.TheTVDBID); value != "" {
		series.TheTVDBID = value
	}
	series.UpdatedAt = time.Now()
	return &preparedScrapedSeries{
		row:            *series,
		removePoster:   removePoster,
		removeBackdrop: removeBackdrop,
	}, nil
}

func (s *ScraperService) findScrapedSeries(ctx context.Context, media *model.Media, match *Match) (*model.Series, error) {
	if media == nil || match == nil {
		return nil, nil
	}
	var series model.Series
	q := s.repo.DB.WithContext(ctx).Model(&model.Series{})
	if id := strings.TrimSpace(media.SeriesID); id != "" && len(id) <= 36 {
		q = q.Where("id = ?", id)
	} else {
		q = q.Where("library_id = ? AND tm_db_id = ?", media.LibraryID, match.TMDbID)
	}
	result := q.Order("updated_at DESC").Limit(1).Find(&series)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		return &series, nil
	}
	return nil, nil
}

func scrapedSeriesMetadataID(libraryID string, match *Match) string {
	identity := fmt.Sprintf("tmdb:%d", match.TMDbID)
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("mediastationgo:series:"+strings.TrimSpace(libraryID)+":"+identity)).String()
}

func savePreparedScrapedSeries(tx *gorm.DB, prepared *preparedScrapedSeries) error {
	if tx == nil || prepared == nil {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"library_id", "title", "poster_url", "backdrop_url", "overview",
			"rating", "year", "tm_db_id", "bangumi_id", "douban_id",
			"thetvdb_id", "updated_at", "deleted_at",
		}),
	}).Create(&prepared.row).Error
}
