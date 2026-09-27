package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type DiscoverPreferenceRepository struct{ db *gorm.DB }

func (r *DiscoverPreferenceRepository) FindByUserID(ctx context.Context, userID string) (*model.UserDiscoverPreference, error) {
	var row model.UserDiscoverPreference
	err := withSQLiteBusyRetry(ctx, func() error {
		row = model.UserDiscoverPreference{}
		return r.db.WithContext(ctx).Where("user_id = ?", userID).First(&row).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *DiscoverPreferenceRepository) Upsert(ctx context.Context, preference *model.UserDiscoverPreference) error {
	return withSQLiteBusyRetry(ctx, func() error {
		var existing model.UserDiscoverPreference
		err := r.db.WithContext(ctx).Where("user_id = ?", preference.UserID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return r.db.WithContext(ctx).Create(preference).Error
		}
		if err != nil {
			return err
		}
		preference.ID = existing.ID
		return r.db.WithContext(ctx).Model(&existing).
			Select("SelectedSections", "AdultFD2PPVSort").Updates(preference).Error
	})
}
