package database

import "gorm.io/gorm"

// Peer invalidation runs after the metadata statement has written all of its
// rows. Updating peers in a BEFORE trigger can revisit a later target of a
// batch UPDATE and raise PostgreSQL's triggered-data-change violation.
func ensureMediaSeriesBindingInvalidation(db *gorm.DB) error {
	for _, statement := range []string{
		`CREATE OR REPLACE FUNCTION mark_media_series_binding_dirty() RETURNS trigger AS $$
BEGIN
  IF OLD.series_binding_scope <> '' AND
     (OLD.library_id, OLD.library_root_id, OLD.path, OLD.series_id, OLD.part_group_key, OLD.season_num, OLD.episode_num,
      OLD.scrape_status, OLD.tm_db_id, OLD.bangumi_id, OLD.douban_id, OLD.thetvdb_id, OLD.deleted_at)
     IS DISTINCT FROM
     (NEW.library_id, NEW.library_root_id, NEW.path, NEW.series_id, NEW.part_group_key, NEW.season_num, NEW.episode_num,
      NEW.scrape_status, NEW.tm_db_id, NEW.bangumi_id, NEW.douban_id, NEW.thetvdb_id, NEW.deleted_at) THEN
    UPDATE media SET series_binding_key = '', series_key_version = 0, emby_key_version = 0
      WHERE series_binding_scope = OLD.series_binding_scope;
  END IF;
  RETURN NULL;
END;
$$ LANGUAGE plpgsql`,
		`DROP TRIGGER IF EXISTS media_series_binding_dirty ON media`,
		`CREATE TRIGGER media_series_binding_dirty
AFTER UPDATE OF library_id, library_root_id, path, series_id, part_group_key, season_num, episode_num,
  scrape_status, tm_db_id, bangumi_id, douban_id, thetvdb_id, deleted_at ON media
FOR EACH ROW EXECUTE FUNCTION mark_media_series_binding_dirty()`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
