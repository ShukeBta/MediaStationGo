package database

import "gorm.io/gorm"

func ensureEmbyKeySchema(db *gorm.DB) error {
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_series_emby_metadata_active ON series(library_id, tm_db_id, updated_at DESC, id DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_emby_series_active ON media(library_id, emby_series_key, created_at DESC, id DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_emby_list_active ON media(library_id, emby_list_key, release_date DESC, year DESC, created_at DESC, id DESC) WHERE deleted_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS idx_media_emby_dirty ON media(id) WHERE deleted_at IS NULL AND emby_key_version <> 1`,
	}
	if isSQLite(db) {
		statements = append(statements, `DROP TRIGGER IF EXISTS media_emby_key_dirty`, `CREATE TRIGGER media_emby_key_dirty
AFTER UPDATE OF library_id, series_id, title, original_name, path, part_group_key, release_date, relative_path, genres, languages, countries, nsfw ON media
WHEN OLD.library_id IS NOT NEW.library_id OR OLD.series_id IS NOT NEW.series_id
OR OLD.title IS NOT NEW.title OR OLD.original_name IS NOT NEW.original_name
OR OLD.path IS NOT NEW.path OR OLD.part_group_key IS NOT NEW.part_group_key
OR OLD.release_date IS NOT NEW.release_date OR OLD.relative_path IS NOT NEW.relative_path
OR OLD.genres IS NOT NEW.genres OR OLD.languages IS NOT NEW.languages OR OLD.countries IS NOT NEW.countries OR OLD.nsfw IS NOT NEW.nsfw
BEGIN UPDATE media SET emby_key_version = 0 WHERE id = NEW.id; END`)
	} else if isPostgres(db) {
		statements = append(statements, `CREATE OR REPLACE FUNCTION mark_media_emby_key_dirty() RETURNS trigger AS $$
BEGIN
 IF (OLD.library_id, OLD.series_id, OLD.title, OLD.original_name, OLD.path, OLD.part_group_key, OLD.release_date, OLD.relative_path, OLD.genres, OLD.languages, OLD.countries, OLD.nsfw)
 IS DISTINCT FROM (NEW.library_id, NEW.series_id, NEW.title, NEW.original_name, NEW.path, NEW.part_group_key, NEW.release_date, NEW.relative_path, NEW.genres, NEW.languages, NEW.countries, NEW.nsfw) THEN
 NEW.emby_key_version = 0;
 END IF;
 RETURN NEW;
END; $$ LANGUAGE plpgsql`,
			`DROP TRIGGER IF EXISTS media_emby_key_dirty ON media`,
			`CREATE TRIGGER media_emby_key_dirty BEFORE UPDATE OF library_id, series_id, title, original_name, path, part_group_key, release_date, relative_path, genres, languages, countries, nsfw ON media FOR EACH ROW EXECUTE FUNCTION mark_media_emby_key_dirty()`)
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}
