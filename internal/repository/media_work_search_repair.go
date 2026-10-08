package repository

import (
	"context"
	"errors"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// BackfillWorkSearchNamesFiltered repairs only the missing/stale episode work
// names that block this query. It does not spend the request budget on hidden
// libraries, deleted rows, or unrelated Emby configuration changes.
func (r *MediaRepository) BackfillWorkSearchNamesFiltered(ctx context.Context, libraryIDs []string, filter MediaQueryFilter, limit int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("media repository unavailable")
	}
	if limit <= 0 || limit > 500 {
		limit = 500
	}
	var count int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []model.Media
		q := tx.Model(&model.Media{}).Select("media.id").Where(incompleteWorkSearchNameSQL, EmbyKeyVersion)
		q = applySeriesGroupScope(q, "media", libraryIDs, filter).Order("media.id ASC").Limit(limit)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if r.embyKeyFunc == nil {
			return errors.New("Emby work title calculator unavailable")
		}
		ids := make([]string, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		if err := r.RefreshEmbyKeys(ctx, tx, ids); err != nil {
			return err
		}
		count = int64(len(ids))
		return nil
	})
	return count, err
}
