package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"gorm.io/gorm"
)

func applyEmbyBrowseGenres(q *gorm.DB, p ItemsParams) *gorm.DB {
	if !hasEmbyGenreFilter(p) {
		return q
	}
	names, valid := embyGenreNames(p)
	if !valid || len(names) == 0 {
		return q.Where("1 = 0")
	}
	for i := range names {
		names[i] = strings.ToLower(names[i])
	}
	if q.Dialector.Name() == "postgres" {
		return q.Where("EXISTS (SELECT 1 FROM jsonb_array_elements_text(CAST(media.emby_genres AS jsonb)) AS genre(value) WHERE genre.value IN ?)", names)
	}
	return q.Where("EXISTS (SELECT 1 FROM json_each(media.emby_genres) AS genre WHERE genre.value IN ?)", names)
}

func embySeriesSQLOrder(p ItemsParams, dialect string) string {
	name := "sort_name"
	id := "group_key"
	if dialect == "postgres" {
		name += ` COLLATE "C"`
		id += ` COLLATE "C"`
	}
	columns := map[string]string{"name": name, "created": "sort_created", "premiere": "sort_premiere", "year": "sort_year", "rating": "sort_rating"}
	missing := []string{}
	order := []string{}
	for _, term := range embySeriesSortTerms(p) {
		column := columns[term.field]
		priorMissing := strings.Join(missing, " OR ")
		if term.field != "name" {
			bucket := "CASE WHEN " + column + " IS NULL THEN 1 ELSE 0 END"
			if priorMissing != "" {
				bucket = "CASE WHEN " + priorMissing + " THEN 0 ELSE " + bucket + " END"
			}
			order = append(order, bucket+" ASC")
		}
		value := column
		if priorMissing != "" {
			value = "CASE WHEN " + priorMissing + " THEN NULL ELSE " + column + " END"
		}
		direction := " ASC"
		if term.descending {
			direction = " DESC"
		}
		order = append(order, value+direction)
		if term.field != "name" {
			missing = append(missing, column+" IS NULL")
		}
	}
	order = append(order, name+" ASC", id+" ASC")
	return strings.Join(order, ", ")
}

// Rank narrow scalar columns in SQL, select group identities on the requested
// page, then load only those groups' complete media rows. No inventory is
// materialized in Go, including searches and genre-filtered pages.
func (e *EmbyService) seriesPageSQL(ctx context.Context, libraryID string, p ItemsParams) (map[string]any, error) {
	return e.seriesPageSQLMode(ctx, libraryID, p, false)
}

func (e *EmbyService) seriesPageSQLMode(ctx context.Context, libraryID string, p ItemsParams, latest bool) (map[string]any, error) {
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("(season_num > 0 OR episode_num > 0 OR COALESCE(part_group_key, '') <> '')")
	keyColumn := "emby_list_key"
	if latest {
		q = q.Where("season_num > 0 OR episode_num > 0")
		keyColumn = "emby_series_key"
	}
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if libraryID != "" {
		q = q.Where("library_id IN ?", e.mergedLibraryIDs(ctx, libraryID))
	}
	q = applyEmbyMediaSearch(q, p)
	if containsEmbyFilter(p.Filters, "IsFavorite") {
		if strings.TrimSpace(p.UserID) == "" {
			return emptyItemsEnvelope(p.StartIndex), nil
		}
		q = q.Where("EXISTS (SELECT 1 FROM favorites f WHERE f.media_id = media.id AND f.user_id = ? AND f.deleted_at IS NULL)", p.UserID)
	}
	if err := e.ensureEmbyKeys(ctx, q); err != nil {
		return nil, err
	}
	q = applyEmbyBrowseGenres(q, p)
	var total int64
	if err := q.Session(&gorm.Session{}).Distinct(keyColumn).Count(&total).Error; err != nil {
		return nil, err
	}
	if p.StartIndex < 0 {
		p.StartIndex = 0
	}
	if p.Limit <= 0 {
		p.Limit = int(total)
	}
	if total == 0 || int64(p.StartIndex) >= total {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": int(total), "StartIndex": p.StartIndex}, nil
	}
	aggregates := q.Session(&gorm.Session{}).Select(keyColumn + ` AS group_key, COUNT(*) AS episode_count,
MAX(CASE WHEN COALESCE(part_group_key, '') <> '' THEN 1 ELSE 0 END) AS is_multipart,
MAX(media.created_at) AS added_at, MAX(NULLIF(media.emby_premiere_date, '')) AS premiere,
MAX(CASE WHEN media.year > 0 THEN media.year ELSE NULL END) AS production_year`).Group(keyColumn)
	anchorOrder := `
CASE WHEN COALESCE(part_group_key, '') <> '' THEN CASE WHEN part_index > 0 THEN part_index ELSE 2147483647 END ELSE 0 END ASC,
CASE WHEN COALESCE(part_group_key, '') <> '' THEN CASE WHEN LOWER(TRIM(path)) LIKE 'cloud://%' OR LOWER(TRIM(strm_url)) LIKE '%/api/cloud/play/%' THEN 1 ELSE 0 END ELSE 0 END ASC,
CASE WHEN COALESCE(part_group_key, '') <> '' THEN width * height ELSE 0 END DESC,
CASE WHEN COALESCE(part_group_key, '') <> '' THEN size_bytes ELSE 0 END DESC,
CASE WHEN COALESCE(part_group_key, '') = '' THEN release_date ELSE '' END DESC,
CASE WHEN COALESCE(part_group_key, '') = '' THEN year ELSE 0 END DESC,
created_at DESC, CASE WHEN COALESCE(part_group_key, '') <> '' THEN part_index ELSE 0 END ASC,
CASE WHEN COALESCE(part_group_key, '') <> '' THEN id ELSE '' END ASC, id DESC`
	if latest {
		anchorOrder = "media.created_at DESC, media.id DESC"
	}
	anchor := q.Session(&gorm.Session{}).Select("media.id").Where(keyColumn + " = ag.group_key").Order(anchorOrder).Limit(1)
	ordinaryOrder := mediaReleaseOrderSQL(true)
	if latest {
		ordinaryOrder = "media.created_at DESC, media.id DESC"
	}
	ordinaryAnchor := q.Session(&gorm.Session{}).Select("media.id").Where(keyColumn + " = ag.group_key").Order(ordinaryOrder).Limit(1)
	partCondition := "COALESCE(sample.part_group_key, '') <> ''"
	if latest {
		partCondition = "1 = 0"
	}
	ratingExpression := "NULLIF(sample.rating, 0)"
	ratingJoin := ""
	anchorExpression := "CASE WHEN ag.is_multipart > 0 THEN (?) ELSE (?) END"
	queryArgs := []any{aggregates, anchor, ordinaryAnchor}
	if latest {
		anchorExpression = "(?)"
		queryArgs = []any{aggregates, ordinaryAnchor}
	}
	for _, term := range embySeriesSortTerms(p) {
		if term.field == "rating" {
			ratingExpression = "CASE WHEN " + partCondition + " THEN NULLIF(sample.rating, 0) ELSE rating_sample.rating END"
			ratingJoin = " LEFT JOIN media rating_sample ON rating_sample.id = (?) "
			rating := q.Session(&gorm.Session{}).Select("media.id").Where(keyColumn + " = ag.group_key AND media.rating > 0").Order(anchorOrder).Limit(1)
			queryArgs = append(queryArgs, rating)
			break
		}
	}
	cte := `WITH aggregates AS (?), cards AS (
SELECT ag.group_key, ag.episode_count,
CASE WHEN ` + partCondition + ` THEN COALESCE(NULLIF(TRIM(sample.part_group_title), ''), sample.title)
ELSE COALESCE(NULLIF(TRIM(metadata.title), ''), sample.emby_series_name) END AS sort_name,
CASE WHEN CAST(ag.added_at AS TEXT) LIKE '0001-%' THEN NULL ELSE ag.added_at END AS sort_created,
ag.premiere AS sort_premiere,
CASE WHEN NOT (` + partCondition + `) AND metadata.year > 0 THEN metadata.year ELSE ag.production_year END AS sort_year,
CASE WHEN NOT (` + partCondition + `) AND metadata.rating > 0 THEN metadata.rating ELSE ` + ratingExpression + ` END AS sort_rating
FROM aggregates ag JOIN media sample ON sample.id = ` + anchorExpression + `
` + ratingJoin + `
LEFT JOIN series metadata ON metadata.id = COALESCE(
(SELECT s.id FROM series s WHERE s.deleted_at IS NULL AND s.id = ag.group_key LIMIT 1),
(SELECT s.id FROM series s WHERE s.deleted_at IS NULL AND sample.tm_db_id > 0 AND s.library_id = sample.library_id AND s.tm_db_id = sample.tm_db_id ORDER BY s.updated_at DESC, s.id DESC LIMIT 1)
)
)
SELECT group_key, episode_count FROM cards ORDER BY `
	queryArgs = append(queryArgs, p.Limit, p.StartIndex)
	var page []embySeriesPageKey
	if err := e.repo.DB.WithContext(ctx).Raw(cte+embySeriesSQLOrder(p, q.Dialector.Name())+" LIMIT ? OFFSET ?", queryArgs...).Scan(&page).Error; err != nil {
		return nil, err
	}
	items, err := e.seriesCardsSQL(ctx, q, page, keyColumn, anchorOrder, latest)
	if err != nil {
		return nil, err
	}
	return map[string]any{"Items": items, "TotalRecordCount": int(total), "StartIndex": p.StartIndex}, nil
}
