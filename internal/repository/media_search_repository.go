package repository

import (
	"context"
	"strings"
	"unicode"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// Search runs a LIKE search against the title field. Empty query returns the
// most recently added items.
func (r *MediaRepository) Search(ctx context.Context, query string, limit int) ([]model.Media, error) {
	return r.SearchFiltered(ctx, query, limit, MediaQueryFilter{IncludeNSFW: true})
}

func (r *MediaRepository) SearchFiltered(ctx context.Context, query string, limit int, filter MediaQueryFilter) ([]model.Media, error) {
	items, _, err := r.SearchFilteredPage(ctx, query, 0, limit, filter)
	return items, err
}

func (r *MediaRepository) SearchFilteredPage(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	query = strings.TrimSpace(query)
	if limit <= 0 {
		limit = 50
	}
	if query != "" && r.searchBackend != nil {
		if items, total, ok := r.searchFilteredBackend(ctx, query, offset, limit, filter); ok {
			return items, total, nil
		}
	}
	if query != "" {
		if items, total, ok := r.searchFilteredFTS(ctx, query, offset, limit, filter); ok {
			if total > 0 {
				return items, total, nil
			}
		}
	}
	return r.searchFilteredLIKE(ctx, query, offset, limit, filter)
}

func (r *MediaRepository) searchFilteredBackend(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, bool) {
	ids, total, err := r.searchBackend.SearchMediaIDs(ctx, query, offset, limit, filter)
	if err != nil {
		return nil, 0, false
	}
	if len(ids) == 0 {
		return []model.Media{}, total, true
	}
	var rows []model.Media
	q := r.db.WithContext(ctx).Model(&model.Media{}).Where("id IN ?", ids)
	q = applyMediaQueryFilter(q, filter)
	if err := q.Find(&rows).Error; err != nil {
		return nil, 0, false
	}
	byID := make(map[string]model.Media, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	items := make([]model.Media, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id]; ok {
			items = append(items, row)
		}
	}
	if len(items) == 0 && total > 0 {
		return nil, 0, false
	}
	return items, total, true
}

func (r *MediaRepository) searchFilteredFTS(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, bool) {
	if !r.searchIndexEnabled(ctx) {
		return nil, 0, false
	}
	ftsQuery := mediaFTSQuery(query)
	if ftsQuery == "" {
		return nil, 0, false
	}
	var total int64
	var items []model.Media
	q := r.db.WithContext(ctx).
		Table("media").
		Joins("JOIN media_search_fts ON media_search_fts.rowid = media.rowid").
		Where("media.deleted_at IS NULL").
		Where("media_search_fts MATCH ?", ftsQuery)
	q = applyQualifiedMediaQueryFilter(q, filter)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, false
	}
	if total == 0 {
		return items, 0, true
	}
	err := q.Select("media.*").Order("bm25(media_search_fts), media.created_at DESC").Offset(offset).Limit(limit).Find(&items).Error
	if err != nil {
		return nil, 0, false
	}
	return items, total, true
}

func (r *MediaRepository) searchFilteredLIKE(ctx context.Context, query string, offset, limit int, filter MediaQueryFilter) ([]model.Media, int64, error) {
	var items []model.Media
	var total int64
	q := r.db.WithContext(ctx).Model(&model.Media{})
	q = applyMediaQueryFilter(q, filter)
	terms := mediaSearchTerms(query)
	for _, term := range terms {
		like := "%" + escapeLike(term) + "%"
		q = q.Where(
			"(title LIKE ? ESCAPE '\\' OR original_name LIKE ? ESCAPE '\\' OR path LIKE ? ESCAPE '\\' OR genres LIKE ? ESCAPE '\\')",
			like, like, like, like,
		)
	}
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if query != "" {
		prefix := escapeLike(query) + "%"
		exact := query
		q = q.Order(gorm.Expr(
			"CASE WHEN title = ? THEN 0 WHEN original_name = ? THEN 1 WHEN title LIKE ? ESCAPE '\\' THEN 2 WHEN original_name LIKE ? ESCAPE '\\' THEN 3 ELSE 4 END, created_at desc",
			exact, exact, prefix, prefix,
		))
	} else {
		q = q.Order("created_at desc")
	}
	err := q.Offset(offset).Limit(limit).Find(&items).Error
	return items, total, err
}

func applyQualifiedMediaQueryFilter(q *gorm.DB, filter MediaQueryFilter) *gorm.DB {
	if !filter.IncludeNSFW {
		q = q.Where("media.nsfw = ?", false)
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		q = q.Where("media.library_id NOT IN ?", filter.HiddenLibraryIDs)
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		q = q.Where("media.library_id IN ?", filter.AllowedLibraryIDs)
	}
	return q
}

func mediaFTSQuery(query string) string {
	terms := mediaSearchTerms(query)
	if len(terms) == 0 {
		return ""
	}
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.ReplaceAll(term, `"`, `""`)
		if term != "" {
			quoted = append(quoted, `"`+term+`"`)
		}
	}
	return strings.Join(quoted, " AND ")
}

func mediaSearchTerms(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		lower := strings.ToLower(field)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		out = append(out, field)
	}
	return out
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func (r *MediaRepository) BackfillSearchIndex(ctx context.Context, batchLimit int) (int64, error) {
	if backend, ok := r.searchBackend.(MediaSearchSyncBackend); ok {
		return r.backfillExternalSearchIndex(ctx, backend, batchLimit)
	}
	if batchLimit <= 0 {
		batchLimit = 1000
	}
	if !r.searchIndexEnabled(ctx) {
		return 0, nil
	}
	// 关键性能点：FTS5 普通列（含 UNINDEXED）不支持索引查找，按
	// media_id 做 NOT EXISTS 是对 FTS 表的整表扫描，再叠加 ORDER BY
	// 后每个批次都要对全部 media 行探测一遍——大库一次启动回填等于
	// 上百亿次行访问，曾把 CPU 钉满数小时。v2 布局下 FTS 行 rowid 与
	// media.rowid 对齐，NOT EXISTS 走 rowid 点查，且无需排序。
	res := r.db.WithContext(ctx).Exec(`
INSERT INTO media_search_fts(rowid, media_id, title, original_name, path, genres)
SELECT m.rowid, m.id, COALESCE(m.title, ''), COALESCE(m.original_name, ''), COALESCE(m.path, ''), COALESCE(m.genres, '')
FROM media AS m
WHERE m.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM media_search_fts AS f WHERE f.rowid = m.rowid
  )
LIMIT ?
`, batchLimit)
	return res.RowsAffected, res.Error
}

func (r *MediaRepository) backfillExternalSearchIndex(ctx context.Context, backend MediaSearchSyncBackend, batchLimit int) (int64, error) {
	if batchLimit <= 0 {
		batchLimit = 1000
	}
	if err := backend.EnsureIndex(ctx); err != nil {
		return 0, err
	}
	var lastID string
	for {
		var rows []model.Media
		q := r.db.WithContext(ctx).
			Model(&model.Media{}).
			Where("deleted_at IS NULL")
		if lastID != "" {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Order("id ASC").Limit(batchLimit).Find(&rows).Error; err != nil {
			return 0, err
		}
		if len(rows) == 0 {
			return 0, nil
		}
		if err := backend.IndexMedia(ctx, rows); err != nil {
			return 0, err
		}
		lastID = rows[len(rows)-1].ID
		if len(rows) < batchLimit {
			return 0, nil
		}
	}
}

func (r *MediaRepository) searchIndexEnabled(ctx context.Context) bool {
	if r == nil || r.db == nil {
		return false
	}
	if r.db.Dialector == nil || r.db.Dialector.Name() != "sqlite" {
		return false
	}
	r.searchIndexOnce.Do(func() {
		var count int64
		err := r.db.WithContext(ctx).
			Raw(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'media_search_fts'`).
			Scan(&count).Error
		r.searchIndexAvailable = err == nil && count > 0
	})
	return r.searchIndexAvailable
}
