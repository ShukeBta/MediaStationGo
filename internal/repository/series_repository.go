package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// SeriesRepository persists model.Series records.
type SeriesRepository struct{ db *gorm.DB }

// FindByID returns the series or (nil, nil).
func (r *SeriesRepository) FindByID(ctx context.Context, id string) (*model.Series, error) {
	var s model.Series
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&s).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// FindByIDs returns active series rows for the requested IDs.
func (r *SeriesRepository) FindByIDs(ctx context.Context, ids []string) ([]model.Series, error) {
	if len(ids) == 0 {
		return []model.Series{}, nil
	}
	var rows []model.Series
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// List returns all series (ordered by title).
func (r *SeriesRepository) List(ctx context.Context) ([]model.Series, error) {
	var s []model.Series
	err := r.db.WithContext(ctx).Order("title asc").Find(&s).Error
	return s, err
}
