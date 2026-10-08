package repository

import (
	"context"
	"errors"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

var errSeriesBindingPlacementChanged = errors.New("media placement changed while acquiring series binding locks")

func retrySeriesBindingWrite(ctx context.Context, write func() error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = write()
		if !errors.Is(err, errSeriesBindingPlacementChanged) {
			return err
		}
	}
	return err
}

// A competing move may finish while this transaction waits on the old scope.
// Re-read only the seed IDs before writing; retry in a fresh transaction if
// their directory/eligibility changed rather than acquiring locks out of order.
func (r *MediaRepository) readLockedSeriesBindingSeeds(ctx context.Context, db *gorm.DB, before []model.Media) ([]model.Media, error) {
	if db.Dialector.Name() != "postgres" {
		return before, nil
	}
	ids := make([]string, len(before))
	previous := make(map[string]model.Media, len(before))
	for i, row := range before {
		ids[i], previous[row.ID] = row.ID, row
	}
	var rows []model.Media
	if err := db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) != len(before) {
		return nil, errSeriesBindingPlacementChanged
	}
	if err := attachSeriesBindingLibraryPaths(ctx, db, rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		old := previous[row.ID]
		if old.LibraryID != row.LibraryID || old.LibraryRootID != row.LibraryRootID || old.Path != row.Path ||
			r.seriesBindingFunc(old).Scope != r.seriesBindingFunc(row).Scope {
			return nil, errSeriesBindingPlacementChanged
		}
	}
	return rows, nil
}
