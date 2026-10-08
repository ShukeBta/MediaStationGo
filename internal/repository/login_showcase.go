package repository

import (
	"context"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

// LoginShowcaseMedia selects presentation metadata only, never playable sources.
type LoginShowcaseMedia struct {
	ID         string
	Title      string
	Overview   string
	Year       int
	ArtworkURL string
}

func (r *MediaRepository) loginShowcaseQuery(ctx context.Context) *gorm.DB {
	// The showcase displays complete portrait posters. A backdrop is a scene,
	// not a substitute for a missing poster.
	const posterArtwork = "COALESCE(NULLIF(TRIM(media.poster_url), ''), NULLIF(TRIM(media.generated_poster_url), ''))"
	return r.db.WithContext(ctx).Model(&model.Media{}).
		Select("media.id, media.title, media.overview, media.year, "+posterArtwork+" AS artwork_url").
		Joins("JOIN libraries ON libraries.id = media.library_id AND libraries.enabled = ? AND libraries.deleted_at IS NULL", true).
		Where("media.nsfw = ? AND media.is_duplicate = ?", false, false).
		Where("TRIM(media.title) <> ''").
		Where(posterArtwork + " IS NOT NULL")
}

// LoginShowcase returns a bounded random selection from enabled local libraries.
func (r *MediaRepository) LoginShowcase(ctx context.Context) ([]LoginShowcaseMedia, error) {
	order := "RANDOM()"
	if r.db.Dialector.Name() == "mysql" {
		order = "RAND()"
	}
	items := make([]LoginShowcaseMedia, 0, 8)
	err := r.loginShowcaseQuery(ctx).Order(order).Limit(8).Find(&items).Error
	return items, err
}

// LoginShowcaseArtwork rechecks eligibility so removed/disabled entries stop serving.
func (r *MediaRepository) LoginShowcaseArtwork(ctx context.Context, id string) (string, error) {
	var item LoginShowcaseMedia
	err := r.loginShowcaseQuery(ctx).Where("media.id = ?", id).Take(&item).Error
	return item.ArtworkURL, err
}
