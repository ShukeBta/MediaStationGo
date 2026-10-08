package repository

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

// SeriesBinding describes a concrete same-library work directory. Key is only
// the row's own confirmed identity, never an identity inherited from a peer.
type SeriesBinding struct {
	Scope     string
	Directory string
	Key       string
}

func (r *MediaRepository) SetSeriesBindingFunc(fn func(model.Media) SeriesBinding) {
	if r != nil {
		r.seriesBindingFunc = fn
	}
}

func mediaSeriesBindingInputsChanged(updates map[string]any) bool {
	for _, column := range []string{"library_id", "library_root_id", "path", "series_id", "part_group_key", "season_num", "episode_num", "scrape_status", "tm_db_id", "bangumi_id", "douban_id", "thetvdb_id", "deleted_at"} {
		if _, ok := updates[column]; ok {
			return true
		}
	}
	return false
}

// SeriesBindingCompatible prevents an inherited identity from overriding an
// explicit identifier, including an unconfirmed ID awaiting scraper review.
func SeriesBindingCompatible(row model.Media, key string) bool {
	provider, id, ok := strings.Cut(key, ":")
	if !ok || id == "" {
		return false
	}
	switch provider {
	case "tmdb":
		return row.TMDbID == 0 || fmt.Sprint(row.TMDbID) == id
	case "bgm":
		return row.BangumiID == 0 || fmt.Sprint(row.BangumiID) == id
	case "douban":
		return row.DoubanID == "" || strings.TrimSpace(row.DoubanID) == id
	case "thetvdb":
		return row.TheTVDBID == "" || strings.TrimSpace(row.TheTVDBID) == id
	case "series":
		return row.SeriesID == "" || strings.TrimSpace(row.SeriesID) == id
	}
	return false
}

// RefreshSeriesBindings updates only the named rows' concrete work directories
// and any previous binding scopes. Prefix ranges include legacy rows lacking a
// scope; the path index avoids a library-wide materialization during repair.
func (r *MediaRepository) RefreshSeriesBindings(ctx context.Context, tx *gorm.DB, ids []string) error {
	if r.seriesBindingFunc == nil || len(ids) == 0 {
		return nil
	}
	var seeds []model.Media
	if err := tx.WithContext(ctx).Unscoped().Where("id IN ?", ids).Find(&seeds).Error; err != nil {
		return err
	}
	if err := attachSeriesBindingLibraryPaths(ctx, tx, seeds); err != nil {
		return err
	}
	if err := r.lockSeriesBindingMedia(ctx, tx, seeds); err != nil {
		return err
	}
	var err error
	seeds, err = r.readLockedSeriesBindingSeeds(ctx, tx, seeds)
	if err != nil {
		return err
	}
	where := tx.Session(&gorm.Session{NewDB: true}).Where("id IN ?", ids)
	seen := make(map[string]bool)
	for _, seed := range seeds {
		if scope := seed.SeriesBindingScope; scope != "" && !seen[scope] {
			where = where.Or("series_binding_scope = ?", scope)
			seen[scope] = true
		}
		binding := r.seriesBindingFunc(seed)
		if binding.Scope == "" || seen["path:"+binding.Scope] {
			continue
		}
		seen["path:"+binding.Scope] = true
		if !seen[binding.Scope] {
			where = where.Or("series_binding_scope = ?", binding.Scope)
		}
		seen[binding.Scope] = true
		for _, prefix := range []string{binding.Directory + "/", strings.ReplaceAll(binding.Directory+"/", "/", "\\")} {
			upper := prefix[:len(prefix)-1] + string(prefix[len(prefix)-1]+1)
			where = where.Or("library_id = ? AND path >= ? AND path < ?", seed.LibraryID, prefix, upper)
		}
	}
	var rows []model.Media
	if err := tx.WithContext(ctx).Unscoped().Where(where).Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}
	if err := attachSeriesBindingLibraryPaths(ctx, tx, rows); err != nil {
		return err
	}
	seedIDs := make(map[string]bool, len(ids))
	for _, id := range ids {
		seedIDs[id] = true
	}
	selected := rows[:0]
	for _, row := range rows {
		if seedIDs[row.ID] || seen[row.SeriesBindingScope] || seen[r.seriesBindingFunc(row).Scope] {
			selected = append(selected, row)
		}
	}
	rows = selected
	bindings := make([]SeriesBinding, len(rows))
	identities := make(map[string]string)
	conflicts := make(map[string]bool)
	providers := make(map[string]map[string]string)
	for i, row := range rows {
		if row.DeletedAt.Valid {
			continue
		}
		binding := r.seriesBindingFunc(row)
		bindings[i] = binding
		if binding.Scope == "" || binding.Key == "" {
			continue
		}
		if prior := identities[binding.Scope]; prior != "" && prior != binding.Key {
			conflicts[binding.Scope] = true
		}
		identities[binding.Scope] = binding.Key
		if providers[binding.Scope] == nil {
			providers[binding.Scope] = make(map[string]string)
		}
		for provider, id := range seriesBindingProviderIDs(row) {
			if prior := providers[binding.Scope][provider]; prior != "" && prior != id {
				conflicts[binding.Scope] = true
			}
			providers[binding.Scope][provider] = id
		}
	}
	changed := make([]string, 0)
	for i := range rows {
		row, binding := &rows[i], bindings[i]
		key := identities[binding.Scope]
		if conflicts[binding.Scope] || !SeriesBindingCompatible(*row, key) {
			key = ""
		}
		for provider, id := range seriesBindingProviderIDs(*row) {
			if confirmed := providers[binding.Scope][provider]; confirmed != "" && confirmed != id {
				key = ""
			}
		}
		bindingChanged := row.SeriesBindingScope != binding.Scope || row.SeriesBindingKey != key
		row.SeriesBindingScope, row.SeriesBindingKey = binding.Scope, key
		// Keep physical projection semantics unchanged; library display paths
		// were loaded only to reject flat library roots as binding scopes.
		row.LibraryPath = ""
		previousKey, previousVersion := row.SeriesKey, row.SeriesKeyVersion
		r.PrepareSeriesKey(row)
		if !bindingChanged && previousKey == row.SeriesKey && previousVersion == row.SeriesKeyVersion {
			continue
		}
		if bindingChanged {
			if err := tx.WithContext(ctx).Unscoped().Model(&model.Media{}).Where("id = ?", row.ID).UpdateColumns(map[string]any{
				"series_binding_scope": binding.Scope, "series_binding_key": key,
			}).Error; err != nil {
				return err
			}
		}
		if err := tx.WithContext(ctx).Unscoped().Model(&model.Media{}).Where("id = ?", row.ID).UpdateColumns(map[string]any{
			"series_key": row.SeriesKey, "series_key_version": row.SeriesKeyVersion,
		}).Error; err != nil {
			return err
		}
		changed = append(changed, row.ID)
	}
	return r.RefreshEmbyKeys(ctx, tx, changed)
}

func attachSeriesBindingLibraryPaths(ctx context.Context, db *gorm.DB, rows []model.Media) error {
	if !db.Migrator().HasTable(&model.Library{}) {
		return nil
	}
	ids := make(map[string]bool)
	for _, row := range rows {
		if row.LibraryID != "" {
			ids[row.LibraryID] = true
		}
	}
	if len(ids) == 0 {
		return nil
	}
	libraryIDs := make([]string, 0, len(ids))
	for id := range ids {
		libraryIDs = append(libraryIDs, id)
	}
	var libraries []model.Library
	if err := db.WithContext(ctx).Select("id", "path").Where("id IN ?", libraryIDs).Find(&libraries).Error; err != nil {
		return err
	}
	paths := make(map[string]string, len(libraries))
	for _, library := range libraries {
		paths[library.ID] = library.Path
	}
	rootIDs := make([]string, 0)
	seenRoots := make(map[string]bool)
	for _, row := range rows {
		if row.LibraryRootID != "" && !seenRoots[row.LibraryRootID] {
			rootIDs = append(rootIDs, row.LibraryRootID)
			seenRoots[row.LibraryRootID] = true
		}
	}
	roots := make(map[string]model.LibraryRoot)
	if len(rootIDs) > 0 && db.Migrator().HasTable(&model.LibraryRoot{}) {
		var stored []model.LibraryRoot
		if err := db.WithContext(ctx).Select("id", "library_id", "path").Where("id IN ?", rootIDs).Find(&stored).Error; err != nil {
			return err
		}
		for _, root := range stored {
			roots[root.ID] = root
		}
	}
	for i := range rows {
		rows[i].LibraryPath = paths[rows[i].LibraryID]
		if root, ok := roots[rows[i].LibraryRootID]; ok && root.LibraryID == rows[i].LibraryID {
			rows[i].LibraryPath = root.Path
		}
	}
	return nil
}

// Take the directory locks before a metadata write takes its first row lock.
// All writers use the same order, so two episodes cannot deadlock while each
// tries to refresh the other episode's inherited identity.
func (r *MediaRepository) lockSeriesBindingRows(ctx context.Context, db *gorm.DB, ids []string, updates map[string]any) error {
	if r.seriesBindingFunc == nil || db.Dialector.Name() != "postgres" {
		return nil
	}
	var before []model.Media
	if err := db.WithContext(ctx).Unscoped().Where("id IN ?", ids).Find(&before).Error; err != nil {
		return err
	}
	if err := attachSeriesBindingLibraryPaths(ctx, db, before); err != nil {
		return err
	}
	rows := append([]model.Media(nil), before...)
	for _, row := range before {
		if library, ok := updates["library_id"].(string); ok {
			row.LibraryID = library
		}
		if path, ok := updates["path"].(string); ok {
			row.Path = path
		}
		if root, ok := updates["library_root_id"].(string); ok {
			row.LibraryRootID = root
		}
		rows = append(rows, row)
	}
	if err := r.lockSeriesBindingMedia(ctx, db, rows); err != nil {
		return err
	}
	_, err := r.readLockedSeriesBindingSeeds(ctx, db, before)
	return err
}

func (r *MediaRepository) lockSeriesBindingMedia(ctx context.Context, db *gorm.DB, rows []model.Media) error {
	if r.seriesBindingFunc == nil || db.Dialector.Name() != "postgres" {
		return nil
	}
	if err := attachSeriesBindingLibraryPaths(ctx, db, rows); err != nil {
		return err
	}
	unique := make(map[string]bool)
	for _, row := range rows {
		if row.SeriesBindingScope != "" {
			unique[row.SeriesBindingScope] = true
		}
		// A metadata write can turn an unclassified file into an episode or
		// detach an explicit group; reserve that possible directory as well.
		row.EpisodeNum, row.PartGroupKey = 1, ""
		if scope := r.seriesBindingFunc(row).Scope; scope != "" {
			unique[scope] = true
		}
	}
	scopes := make([]string, 0, len(unique))
	for scope := range unique {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	for _, scope := range scopes {
		hash := sha256.Sum256([]byte("media-series-binding:" + scope))
		lock := int64(binary.BigEndian.Uint64(hash[:8]))
		if err := db.WithContext(ctx).Exec("SELECT pg_advisory_xact_lock(?)", lock).Error; err != nil {
			return err
		}
	}
	return nil
}

func seriesBindingProviderIDs(row model.Media) map[string]string {
	ids := make(map[string]string)
	if row.TMDbID > 0 {
		ids["tmdb"] = fmt.Sprint(row.TMDbID)
	}
	if row.BangumiID > 0 {
		ids["bgm"] = fmt.Sprint(row.BangumiID)
	}
	if row.DoubanID != "" {
		ids["douban"] = row.DoubanID
	}
	if row.TheTVDBID != "" {
		ids["thetvdb"] = row.TheTVDBID
	}
	return ids
}
