package service

import (
	"context"
	"fmt"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// InitializeBrowseKeys completes the upgrade before HTTP starts. Subsequent
// starts only check for missing/invalid projections; there is no periodic full
// rewrite and no cloud or scraper call in this migration.
func (e *EmbyService) InitializeBrowseKeys(ctx context.Context) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := e.repo.Media.BackfillEmbyKeys(ctx, 1000)
		if err != nil {
			return total, err
		}
		total += n
		if n == 0 {
			return total, nil
		}
	}
}

// Repair only the scoped invalid identities, never scan/group the library in
// Go as a query fallback. Normal upserts keep these identities current inline.
func (e *EmbyService) ensureEmbyKeys(ctx context.Context, scope *gorm.DB) error {
	return e.repo.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var stale []model.Media
		// Keep the visibility/search scope, but avoid a self semi-join over all
		// visible media. Bind the cloned statement to this transaction so the
		// stale-row lock and subsequent repair retain their atomicity.
		q := scope.WithContext(ctx).Select("media.id").Where("emby_key_version <> ? OR emby_series_key IS NULL OR emby_series_key = '' OR emby_list_key IS NULL OR emby_list_key = '' OR COALESCE(emby_config_key, '') <> ?", repository.EmbyKeyVersion, e.embyBrowseConfigKey())
		q.Statement.ConnPool = tx.Statement.ConnPool
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := q.Limit(501).Find(&stale).Error; err != nil {
			return err
		}
		if len(stale) > 500 {
			return fmt.Errorf("Emby identities are not ready: more than 500 scoped rows require migration")
		}
		ids := make([]string, len(stale))
		for i := range stale {
			ids[i] = stale[i].ID
		}
		return e.repo.Media.RefreshEmbyKeys(ctx, tx, ids)
	})
}
