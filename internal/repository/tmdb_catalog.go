package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type TMDbCatalogRepository struct{ db *gorm.DB }

func NewTMDbCatalogRepository(db *gorm.DB) *TMDbCatalogRepository {
	return &TMDbCatalogRepository{db: db}
}

func (r *TMDbCatalogRepository) Find(ctx context.Context, key string) (*model.TMDbCatalogItem, error) {
	var item model.TMDbCatalogItem
	err := r.db.WithContext(ctx).First(&item, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

// Save preserves complete snapshots when a later root/season outline is poorer.
// Entries in a season are written together: partial responses never leave half a catalog.
func (r *TMDbCatalogRepository) Save(ctx context.Context, items []model.TMDbCatalogItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			if item.Key == "" || item.TMDbID <= 0 || !json.Valid([]byte(item.Snapshot)) {
				return errors.New("invalid TMDB catalog snapshot")
			}
			update := []string{"kind", "root_id", "tm_db_id", "season_num", "episode_num", "title", "overview", "release_date", "poster_url", "still_url", "episode_count", "snapshot", "complete", "fetched_at"}
			conflict := clause.OnConflict{Columns: []clause.Column{{Name: "key"}}, DoUpdates: clause.AssignmentColumns(update)}
			// A plain detail request must not discard appended credits, translations
			// or external IDs already fetched for this exact identity.
			keepExpanded := "tm_db_catalog_items.expanded = ? AND excluded.expanded = ? AND tm_db_catalog_items.tm_db_id = excluded.tm_db_id"
			for i := range conflict.DoUpdates {
				name := conflict.DoUpdates[i].Column.Name
				if name == "snapshot" || name == "fetched_at" {
					conflict.DoUpdates[i].Value = gorm.Expr("CASE WHEN "+keepExpanded+" THEN tm_db_catalog_items."+name+" ELSE excluded."+name+" END", true, false)
				}
			}
			conflict.DoUpdates = append(conflict.DoUpdates, clause.Assignment{Column: clause.Column{Name: "expanded"}, Value: gorm.Expr("CASE WHEN "+keepExpanded+" THEN ? ELSE excluded.expanded END", true, false, true)})
			if !item.Complete {
				conflict.Where = clause.Where{Exprs: []clause.Expression{clause.Eq{Column: clause.Column{Table: "tm_db_catalog_items", Name: "complete"}, Value: false}}}
			}
			if err := tx.Clauses(conflict).Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *TMDbCatalogRepository) Series(ctx context.Context, rootID int) ([]model.TMDbCatalogItem, error) {
	items := []model.TMDbCatalogItem{}
	err := r.db.WithContext(ctx).Where("root_id = ? AND kind IN ?", rootID, []string{"series", "season", "episode"}).Order("season_num, episode_num, kind").Find(&items).Error
	return items, err
}

func (r *TMDbCatalogRepository) State(ctx context.Context, key string) (*model.TMDbCatalogJob, error) {
	var job model.TMDbCatalogJob
	err := r.db.WithContext(ctx).First(&job, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (r *TMDbCatalogRepository) Claim(ctx context.Context, key string, cutoff time.Time, force bool) (*model.TMDbCatalogJob, error) {
	job := model.TMDbCatalogJob{Key: key, Status: "pending", DueAt: cutoff}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&job).Error; err != nil {
		return nil, err
	}
	until, token := time.Now().UTC().Add(2*time.Minute), uuid.NewString()
	query := r.db.WithContext(ctx).Model(&model.TMDbCatalogJob{}).Where("key = ? AND (lease_until IS NULL OR lease_until < ?)", key, time.Now().UTC())
	if !force {
		query = query.Where("due_at <= ?", cutoff)
	}
	res := query.Updates(map[string]any{"status": "running", "lease_token": token, "lease_until": until})
	if res.Error != nil || res.RowsAffected == 0 {
		return nil, res.Error
	}
	claimed, err := r.State(ctx, key)
	if err != nil {
		return nil, err
	}
	if claimed == nil || claimed.LeaseToken != token {
		return nil, nil
	}
	return claimed, nil
}

func (r *TMDbCatalogRepository) Finish(ctx context.Context, job *model.TMDbCatalogJob, status, reason string, due time.Time, success bool) error {
	updates := map[string]any{"status": status, "last_error": reason, "due_at": due, "lease_token": "", "lease_until": nil, "attempts": job.Attempts + 1}
	if success {
		updates["checked_at"] = time.Now().UTC()
		updates["attempts"] = 0
	}
	res := r.db.WithContext(ctx).Model(&model.TMDbCatalogJob{}).Where("key = ? AND lease_token = ?", job.Key, job.LeaseToken).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return errors.New("TMDB catalog lease changed")
	}
	return nil
}
