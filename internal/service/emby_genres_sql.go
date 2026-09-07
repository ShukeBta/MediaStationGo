package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// The classifier has five explicit media types plus metadata-based inference.
// Keep the adult display subtype distinction: only literal "adult" forces AV.
func embyGenreVariant(mediaType string) string {
	if strings.EqualFold(strings.TrimSpace(mediaType), "adult") {
		return "adult"
	}
	left := normalizeMediaType(mediaType, "TV series", "")
	right := normalizeMediaType(mediaType, "movie", "")
	if left != right {
		return "auto"
	}
	if left == "adult" {
		return "nsfw"
	}
	return left
}

func (e *EmbyService) genresSQL(ctx context.Context, p ItemsParams) (map[string]any, error) {
	if p.Limit <= 0 || p.Limit > 500 {
		p.Limit = 50
	}
	if p.StartIndex < 0 {
		p.StartIndex = 0
	}
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{})
	q = e.applyUserMediaVisibility(ctx, q, p.UserID)
	if strings.TrimSpace(p.ParentID) != "" {
		q = q.Where("library_id IN ?", e.mergedLibraryIDs(ctx, p.ParentID))
	}
	movie := containsItemType(p.IncludeItemTypes, "Movie")
	series := containsItemType(p.IncludeItemTypes, "Series")
	if movie && !series {
		q = e.filterMovieItems(ctx, q)
	} else if series && !movie {
		q = e.filterEpisodeItems(ctx, q)
	}
	if err := e.ensureEmbyKeys(ctx, q); err != nil {
		return nil, err
	}
	libraries, err := e.repo.Library.List(ctx)
	if err != nil {
		return nil, err
	}
	variant := "CASE"
	args := []any{}
	for _, lib := range libraries {
		variant += " WHEN media.library_id = ? THEN ?"
		args = append(args, lib.ID, embyGenreVariant(lib.Type))
	}
	if len(libraries) == 0 {
		variant = "'auto'"
	} else {
		variant += " ELSE 'auto' END"
	}
	base := q.Select("media.emby_genre_variants, "+variant+" AS variant", args...)
	expand := `SELECT genre.value AS name FROM inventory m, json_each(json_extract(m.emby_genre_variants, '$.' || m.variant)) genre`
	if q.Dialector.Name() == "postgres" {
		expand = `SELECT genre.value AS name FROM inventory m, jsonb_array_elements_text(CAST(m.emby_genre_variants AS jsonb) -> m.variant) AS genre(value)`
	}
	cte := "WITH inventory AS (?), names AS (" + expand + "), facets AS (SELECT MIN(name) AS name, LOWER(name) AS key, COUNT(*) AS count FROM names GROUP BY LOWER(name)) "
	where := " WHERE 1=1"
	params := []any{base}
	if value := strings.ToLower(strings.TrimSpace(p.SearchTerm)); value != "" {
		where += " AND key LIKE ? ESCAPE '\\'"
		params = append(params, "%"+escapeEmbyLike(value)+"%")
	}
	if value := strings.ToLower(strings.TrimSpace(p.NameStartsWith)); value != "" {
		where += " AND key LIKE ? ESCAPE '\\'"
		params = append(params, escapeEmbyLike(value)+"%")
	}
	var total int64
	if err := e.repo.DB.WithContext(ctx).Raw(cte+"SELECT COUNT(*) FROM facets"+where, params...).Scan(&total).Error; err != nil {
		return nil, err
	}
	direction := "ASC"
	if strings.EqualFold(firstCSVValue(p.SortOrder), "Descending") {
		direction = "DESC"
	}
	params = append(params, p.Limit, p.StartIndex)
	var genres []embyGenreCount
	if err := e.repo.DB.WithContext(ctx).Raw(cte+"SELECT name, count FROM facets"+where+" ORDER BY key "+direction+" LIMIT ? OFFSET ?", params...).Scan(&genres).Error; err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(genres))
	for _, genre := range genres {
		items = append(items, embyGenrePayload(genre))
	}
	return map[string]any{"Items": items, "TotalRecordCount": int(total), "StartIndex": p.StartIndex}, nil
}
