package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const mediaVersionKeyVersion = 1

// MediaVersionGroupPage is the SQL-selected page of effective version-group
// keys. Full media rows are loaded only for these keys afterwards.
type MediaVersionGroupPage struct {
	Key                   string    `gorm:"column:media_version_key"`
	RepresentativeID      string    `gorm:"column:representative_id"`
	RepresentativeCreated time.Time `gorm:"column:representative_created_at"`
	TotalGroups           int64     `gorm:"column:total_groups"`
}

// ListMediaVersionGroupPage lets PostgreSQL select and paginate version
// groups. The primary ordering mirrors betterMediaPart/betterMediaVersion in
// the service; the final response still runs the authoritative Go grouping
// over only the selected groups to preserve the exact payload shape.
func (r *MediaRepository) ListMediaVersionGroupPage(
	ctx context.Context,
	libraryIDs []string,
	filter MediaQueryFilter,
	offset, limit int,
) ([]MediaVersionGroupPage, bool, error) {
	if r == nil || r.db == nil || len(libraryIDs) == 0 {
		return nil, false, nil
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 50
	}

	whereParts := []string{"deleted_at IS NULL", "media_version_key_version = ?", "media_version_key <> ''"}
	args := []any{mediaVersionKeyVersion}
	if len(libraryIDs) == 1 {
		whereParts = append(whereParts, "library_id = ?")
		args = append(args, libraryIDs[0])
	} else {
		whereParts = append(whereParts, "library_id IN ?")
		args = append(args, libraryIDs)
	}
	if !filter.IncludeNSFW {
		whereParts = append(whereParts, "nsfw = ?")
		args = append(args, false)
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		whereParts = append(whereParts, "library_id NOT IN ?")
		args = append(args, filter.HiddenLibraryIDs)
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		whereParts = append(whereParts, "library_id IN ?")
		args = append(args, filter.AllowedLibraryIDs)
	}

	// All rows belonging to one effective key are homogeneous with respect to
	// part_group_key. Local files win over cloud versions, then the existing
	// quality/size/created-at tie breakers choose the representative row.
	const selectSQL = `
WITH ranked AS (
  SELECT media_version_key,
         id,
         created_at,
         ROW_NUMBER() OVER (
           PARTITION BY media_version_key
           ORDER BY
             CASE WHEN COALESCE(part_group_key, '') <> '' AND part_index > 0 THEN 0 ELSE 1 END ASC,
             CASE WHEN COALESCE(part_group_key, '') <> '' AND part_index > 0 THEN part_index ELSE 2147483647 END ASC,
             CASE WHEN (LOWER(COALESCE(path, '')) LIKE 'cloud://%%' OR LOWER(COALESCE(strm_url, '')) LIKE '%%/api/cloud/play/%%') THEN 0 ELSE 1 END DESC,
             (width * height) DESC,
             size_bytes DESC,
             created_at DESC,
             id DESC
         ) AS representative_rank
  FROM media
  WHERE %s
), grouped AS (
	SELECT media_version_key,
	       id,
	       created_at,
         COUNT(*) OVER () AS total_groups
  FROM ranked
  WHERE representative_rank = 1
)
SELECT media_version_key,
       id AS representative_id,
       created_at AS representative_created_at,
       total_groups
FROM grouped
ORDER BY created_at DESC, id DESC
LIMIT ? OFFSET ?`

	var rows []MediaVersionGroupPage
	whereSQL := strings.Join(whereParts, " AND ")
	if err := r.db.WithContext(ctx).Raw(fmt.Sprintf(selectSQL, whereSQL), append(args, limit, offset)...).Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	return rows, true, nil
}

// ListMediaByVersionGroupKeys loads complete rows only for the selected page
// of groups. Callers apply the same visibility filter used for page selection.
func (r *MediaRepository) ListMediaByVersionGroupKeys(
	ctx context.Context,
	keys []string,
	libraryIDs []string,
	filter MediaQueryFilter,
) ([]model.Media, error) {
	if r == nil || r.db == nil || len(keys) == 0 || len(libraryIDs) == 0 {
		return []model.Media{}, nil
	}
	q := r.db.WithContext(ctx).Where("deleted_at IS NULL AND media_version_key_version = ? AND media_version_key IN ?", mediaVersionKeyVersion, keys)
	if len(libraryIDs) == 1 {
		q = q.Where("library_id = ?", libraryIDs[0])
	} else {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	var rows []model.Media
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// CountMediaVersionGroups returns the total number of effective groups for a
// filtered scope, including when a requested page is beyond the end.
func (r *MediaRepository) CountMediaVersionGroups(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) (int64, error) {
	if r == nil || r.db == nil || len(libraryIDs) == 0 {
		return 0, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Media{}).
		Where("deleted_at IS NULL AND media_version_key_version = ? AND media_version_key <> ''", mediaVersionKeyVersion)
	if len(libraryIDs) == 1 {
		q = q.Where("library_id = ?", libraryIDs[0])
	} else {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	var total int64
	if err := q.Distinct("media_version_key").Count(&total).Error; err != nil {
		return 0, err
	}
	return total, nil
}

// MediaVersionKeysComplete reports whether all active rows in the requested
// scope have the current persisted grouping key.
func (r *MediaRepository) MediaVersionKeysComplete(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) (bool, error) {
	if r == nil || r.db == nil || len(libraryIDs) == 0 {
		return false, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Media{}).
		Where("deleted_at IS NULL AND (media_version_key_version <> ? OR media_version_key_version IS NULL OR media_version_key IS NULL OR media_version_key = '')", mediaVersionKeyVersion)
	if len(libraryIDs) == 1 {
		q = q.Where("library_id = ?", libraryIDs[0])
	} else {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	var stale int64
	if err := q.Count(&stale).Error; err != nil {
		return false, err
	}
	return stale == 0, nil
}

// CountMediaVersionKeyStale counts rows requiring the bounded background
// repair. It is intentionally separate from the page query so an incomplete
// projection never silently returns a partial page.
func (r *MediaRepository) CountMediaVersionKeyStale(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) (int64, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Media{}).
		Where("deleted_at IS NULL AND (media_version_key_version <> ? OR media_version_key_version IS NULL OR media_version_key IS NULL OR media_version_key = '')", mediaVersionKeyVersion)
	if len(libraryIDs) > 0 {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}
