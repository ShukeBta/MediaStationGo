package database

import "gorm.io/gorm"

func ensureEmbyKeySchema(db *gorm.DB) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_series_emby_metadata_active ON series(library_id, tm_db_id, updated_at DESC, id DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_emby_series_active ON media(library_id, emby_series_key, created_at DESC, id DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_emby_list_active ON media(library_id, emby_list_key, release_date DESC, year DESC, created_at DESC, id DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_emby_dirty ON media(id) WHERE deleted_at IS NULL AND emby_key_version <> 1`,
		`CREATE INDEX IF NOT EXISTS idx_media_emby_incomplete_v2_active ON media(id) WHERE deleted_at IS NULL AND (emby_key_version <> 1 OR emby_series_key IS NULL OR emby_series_key = '' OR emby_list_key IS NULL OR emby_list_key = '' OR emby_version_key IS NULL OR emby_version_key = '')`,
		`CREATE INDEX IF NOT EXISTS idx_media_emby_config_active ON media(COALESCE(emby_config_key, ''), id) WHERE deleted_at IS NULL`,
	}
	if isSQLite(db) {
		statements = append(statements, `DROP TRIGGER IF EXISTS media_emby_key_dirty`, `CREATE TRIGGER media_emby_key_dirty
AFTER UPDATE OF library_id, series_id, title, original_name, path, part_group_key, release_date, relative_path, genres, languages, countries, nsfw, season_num, episode_num, year, tm_db_id, bangumi_id, title_cleanup_version, version_group_key ON media
WHEN OLD.library_id IS NOT NEW.library_id OR OLD.series_id IS NOT NEW.series_id
OR OLD.title IS NOT NEW.title OR OLD.original_name IS NOT NEW.original_name
OR OLD.path IS NOT NEW.path OR OLD.part_group_key IS NOT NEW.part_group_key
OR OLD.release_date IS NOT NEW.release_date OR OLD.relative_path IS NOT NEW.relative_path
OR OLD.genres IS NOT NEW.genres OR OLD.languages IS NOT NEW.languages OR OLD.countries IS NOT NEW.countries OR OLD.nsfw IS NOT NEW.nsfw
OR OLD.season_num IS NOT NEW.season_num OR OLD.episode_num IS NOT NEW.episode_num OR OLD.year IS NOT NEW.year
OR OLD.tm_db_id IS NOT NEW.tm_db_id OR OLD.bangumi_id IS NOT NEW.bangumi_id
OR OLD.title_cleanup_version IS NOT NEW.title_cleanup_version OR OLD.version_group_key IS NOT NEW.version_group_key
BEGIN UPDATE media SET emby_key_version = 0 WHERE id = NEW.id; END`)
	} else if isPostgres(db) {
		statements = append(statements, `CREATE OR REPLACE FUNCTION mark_media_emby_key_dirty() RETURNS trigger AS $$
BEGIN
 IF (OLD.library_id, OLD.series_id, OLD.title, OLD.original_name, OLD.path, OLD.part_group_key, OLD.release_date, OLD.relative_path, OLD.genres, OLD.languages, OLD.countries, OLD.nsfw,
     OLD.season_num, OLD.episode_num, OLD.year, OLD.tm_db_id, OLD.bangumi_id, OLD.title_cleanup_version, OLD.version_group_key)
 IS DISTINCT FROM (NEW.library_id, NEW.series_id, NEW.title, NEW.original_name, NEW.path, NEW.part_group_key, NEW.release_date, NEW.relative_path, NEW.genres, NEW.languages, NEW.countries, NEW.nsfw,
     NEW.season_num, NEW.episode_num, NEW.year, NEW.tm_db_id, NEW.bangumi_id, NEW.title_cleanup_version, NEW.version_group_key) THEN
 NEW.emby_key_version = 0;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql`,
			`DROP TRIGGER IF EXISTS media_emby_key_dirty ON media`,
			`CREATE TRIGGER media_emby_key_dirty BEFORE UPDATE OF library_id, series_id, title, original_name, path, part_group_key, release_date, relative_path, genres, languages, countries, nsfw, season_num, episode_num, year, tm_db_id, bangumi_id, title_cleanup_version, version_group_key ON media FOR EACH ROW EXECUTE FUNCTION mark_media_emby_key_dirty()`,
			`CREATE INDEX IF NOT EXISTS idx_media_emby_version_representative_v1_active ON media(
  emby_version_key,
  (CASE WHEN COALESCE(part_group_key, '') <> '' AND part_index > 0 THEN 0 ELSE 1 END) ASC,
  (CASE WHEN COALESCE(part_group_key, '') <> '' AND part_index > 0 THEN part_index ELSE 2147483647 END) ASC,
  (CASE WHEN TRIM(COALESCE(strm_url, '')) <> '' OR LOWER(TRIM(COALESCE(path, ''))) LIKE 'cloud://%' THEN 0 ELSE 1 END) DESC,
  width DESC,
  size_bytes DESC,
  created_at DESC,
  id DESC
) INCLUDE (library_id, nsfw, release_date, year, title, rating, updated_at)
WHERE deleted_at IS NULL AND emby_key_version = 1 AND emby_version_key <> ''`)
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
