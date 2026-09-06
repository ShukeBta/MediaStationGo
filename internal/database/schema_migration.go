package database

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// AutoMigrate creates tables for every model registered in the model package.
func AutoMigrate(db *gorm.DB) (err error) {
	aliasTriggerSuspended, err := suspendMediaSearchAliasInvalidation(db)
	if err != nil {
		return err
	}
	defer func() {
		if !aliasTriggerSuspended {
			return
		}
		if restoreErr := ensureMediaSearchAliasInvalidation(db); restoreErr != nil {
			err = errors.Join(err, fmt.Errorf("restore media search alias trigger: %w", restoreErr))
		}
	}()

	resourceImportTableExisted := db.Migrator().HasTable(&model.ResourceImportJob{})
	hadKeepOldVersion := resourceImportTableExisted && db.Migrator().HasColumn(&model.ResourceImportJob{}, "keep_old_version")
	if err := db.AutoMigrate(model.AllModels()...); err != nil {
		return err
	}
	if resourceImportTableExisted && !hadKeepOldVersion {
		if err := db.Model(&model.ResourceImportJob{}).
			Where("upgrade_media_id <> ''").
			Update("keep_old_version", true).Error; err != nil {
			return err
		}
	}
	if err := backfillArchivedSubscriptions(db); err != nil {
		return err
	}
	if err := ensurePostgresColumnCompatibility(db); err != nil {
		return err
	}
	if err := enforceTelegramBindingOneToOne(db); err != nil {
		return err
	}
	if err := ensurePerformanceIndexes(db); err != nil {
		return err
	}
	if err := ensureMediaSeriesKeyInvalidation(db); err != nil {
		return err
	}
	if err := ensureMediaSearchAliasInvalidation(db); err != nil {
		return err
	}
	aliasTriggerSuspended = false
	if err := ensureLibraryRootsCompatibility(db); err != nil {
		return err
	}
	if isSQLite(db) {
		return ensureMediaSearchIndex(db)
	}
	return nil
}

func backfillArchivedSubscriptions(db *gorm.DB) error {
	return db.Model(&model.Subscription{}).Unscoped().
		Where("deleted_at IS NOT NULL AND archived_at IS NULL").
		Updates(map[string]any{
			"enabled":        false,
			"archived_at":    gorm.Expr("deleted_at"),
			"archive_reason": "手动删除",
		}).Error
}

func suspendMediaSearchAliasInvalidation(db *gorm.DB) (bool, error) {
	if !isPostgres(db) || !db.Migrator().HasTable(&model.Media{}) {
		return false, nil
	}
	var exists bool
	if err := db.Raw(`
SELECT EXISTS (
  SELECT 1
  FROM pg_trigger
  WHERE tgname = 'media_search_alias_dirty'
    AND tgrelid = 'media'::regclass
    AND NOT tgisinternal
)`).Scan(&exists).Error; err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	if err := db.Exec(`DROP TRIGGER media_search_alias_dirty ON media`).Error; err != nil {
		return false, err
	}
	return true, nil
}

func ensureMediaSearchAliasInvalidation(db *gorm.DB) error {
	switch {
	case isSQLite(db):
		return db.Exec(`
CREATE TRIGGER IF NOT EXISTS media_search_alias_dirty
AFTER UPDATE OF title, original_name, genres, actors ON media
WHEN new.search_alias_version = old.search_alias_version
BEGIN
  UPDATE media SET search_alias_version = 0 WHERE id = new.id;
END`).Error
	case isPostgres(db):
		for _, stmt := range []string{
			`CREATE OR REPLACE FUNCTION mark_media_search_alias_dirty() RETURNS trigger AS $$
BEGIN
  IF (OLD.title, OLD.original_name, OLD.genres, OLD.actors)
     IS DISTINCT FROM
     (NEW.title, NEW.original_name, NEW.genres, NEW.actors) THEN
    NEW.search_alias_version = 0;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql`,
			`DROP TRIGGER IF EXISTS media_search_alias_dirty ON media`,
			`CREATE TRIGGER media_search_alias_dirty
BEFORE UPDATE OF title, original_name, genres, actors ON media
FOR EACH ROW EXECUTE FUNCTION mark_media_search_alias_dirty()`,
		} {
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func ensurePostgresColumnCompatibility(db *gorm.DB) error {
	if !isPostgres(db) {
		return nil
	}
	statements := []string{
		`ALTER TABLE media ALTER COLUMN container TYPE varchar(128)`,
		`ALTER TABLE media ALTER COLUMN genres TYPE text`,
		`ALTER TABLE media ALTER COLUMN actors TYPE text`,
		`ALTER TABLE media ALTER COLUMN strm_url TYPE text`,
		`ALTER TABLE media ALTER COLUMN series_id TYPE varchar(128)`,
		`ALTER TABLE media ALTER COLUMN duplicate_of TYPE varchar(128)`,
		`ALTER TABLE playback_histories ALTER COLUMN media_id TYPE varchar(128)`,
		`ALTER TABLE favorites ALTER COLUMN media_id TYPE varchar(128)`,
		`ALTER TABLE playlist_items ALTER COLUMN media_id TYPE varchar(128)`,
		`ALTER TABLE strm_records ALTER COLUMN media_id TYPE varchar(128)`,
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensurePerformanceIndexes(db *gorm.DB) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_media_library_created_active ON media(library_id, created_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_library_release_active ON media(library_id, release_date DESC, year DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_library_episode_active ON media(library_id, season_num, episode_num, created_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_library_root_active ON media(library_id, library_root_id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_series_active ON media(series_id, season_num, episode_num) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_library_series_key_active ON media(library_id, series_key, created_at DESC) WHERE deleted_at IS NULL AND series_key_version = 1 AND series_key <> ''`,
		`CREATE INDEX IF NOT EXISTS idx_favorites_user_media_active ON favorites(user_id, media_id) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_playback_histories_user_media_active ON playback_histories(user_id, media_id, watched_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_playback_histories_resume_active ON playback_histories(user_id, completed, watched_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_play_profiles_user_created_active ON play_profiles(user_id, created_at DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_active_created ON refresh_tokens(user_id, created_at DESC, id DESC) WHERE revoked = false`,
	}
	if isSQLite(db) {
		statements = append(statements,
			`CREATE INDEX IF NOT EXISTS idx_media_title_active ON media(title COLLATE NOCASE) WHERE deleted_at IS NULL`,
			`CREATE INDEX IF NOT EXISTS idx_media_original_name_active ON media(original_name COLLATE NOCASE) WHERE deleted_at IS NULL`,
		)
	} else {
		statements = append(statements,
			`CREATE INDEX IF NOT EXISTS idx_media_title_active ON media(title) WHERE deleted_at IS NULL`,
			`CREATE INDEX IF NOT EXISTS idx_media_original_name_active ON media(original_name) WHERE deleted_at IS NULL`,
		)
	}
	for _, stmt := range statements {
		if err := db.Exec(stmt).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureMediaSeriesKeyInvalidation marks the persisted grouping key stale when
// any field that participates in its calculation changes.  The service
// backfill recomputes the key in Go; the trigger keeps direct metadata edits,
// reclassification and library moves safe without duplicating that logic in
// SQL.
func ensureMediaSeriesKeyInvalidation(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.Media{}) {
		return nil
	}
	switch {
	case isSQLite(db):
		return db.Exec(`
CREATE TRIGGER IF NOT EXISTS media_series_key_dirty
AFTER UPDATE OF library_id, series_id, title, original_name, path, season_num, episode_num,
  scrape_status, tm_db_id, bangumi_id, douban_id, thetvdb_id
ON media
WHEN NEW.series_key_version = OLD.series_key_version
BEGIN
  UPDATE media SET series_key_version = 0 WHERE id = NEW.id;
END`).Error
	case isPostgres(db):
		for _, stmt := range []string{
			`CREATE OR REPLACE FUNCTION mark_media_series_key_dirty() RETURNS trigger AS $$
BEGIN
  IF (OLD.library_id, OLD.series_id, OLD.title, OLD.original_name, OLD.path,
      OLD.season_num, OLD.episode_num, OLD.scrape_status,
      OLD.tm_db_id, OLD.bangumi_id, OLD.douban_id, OLD.thetvdb_id)
     IS DISTINCT FROM
     (NEW.library_id, NEW.series_id, NEW.title, NEW.original_name, NEW.path,
      NEW.season_num, NEW.episode_num, NEW.scrape_status,
      NEW.tm_db_id, NEW.bangumi_id, NEW.douban_id, NEW.thetvdb_id) THEN
    NEW.series_key_version = 0;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql`,
			`DROP TRIGGER IF EXISTS media_series_key_dirty ON media`,
			`CREATE TRIGGER media_series_key_dirty
BEFORE UPDATE OF library_id, series_id, title, original_name, path, season_num, episode_num,
  scrape_status, tm_db_id, bangumi_id, douban_id, thetvdb_id ON media
FOR EACH ROW EXECUTE FUNCTION mark_media_series_key_dirty()`,
		} {
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func enforceTelegramBindingOneToOne(db *gorm.DB) error {
	if !db.Migrator().HasTable(&model.TelegramBinding{}) {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`
DELETE FROM telegram_bindings
WHERE deleted_at IS NULL
  AND user_id IN (
    SELECT user_id
    FROM telegram_bindings
    WHERE deleted_at IS NULL
    GROUP BY user_id
    HAVING COUNT(*) > 1
  )
  AND id NOT IN (
    SELECT id
    FROM (
      SELECT id,
             ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY updated_at DESC, created_at DESC, id DESC) AS rn
      FROM telegram_bindings
      WHERE deleted_at IS NULL
    ) AS ranked_bindings
    WHERE rn = 1
  )
`).Error; err != nil {
			return err
		}
		return tx.Exec(`
CREATE UNIQUE INDEX IF NOT EXISTS idx_telegram_bindings_user_id_active
ON telegram_bindings(user_id)
WHERE deleted_at IS NULL
`).Error
	})
}
