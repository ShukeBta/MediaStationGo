package service

import (
	"context"
	"database/sql/driver"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type embySeriesPageKey struct {
	GroupKey     string
	EpisodeCount int
}

type embySQLTime struct{ time.Time }

func (t *embySQLTime) Scan(value any) error {
	if value == nil {
		t.Time = time.Time{}
		return nil
	}
	if v, ok := value.(time.Time); ok {
		t.Time = v
		return nil
	}
	var raw string
	switch v := value.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return fmt.Errorf("unsupported SQL timestamp type %T", value)
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05.999999999-07:00", "2006-01-02 15:04:05.999999999"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			if _, offset := parsed.Zone(); offset == 0 {
				parsed = parsed.UTC()
			}
			t.Time = parsed
			return nil
		}
	}
	return fmt.Errorf("invalid SQL timestamp %q", raw)
}
func (t embySQLTime) Value() (driver.Value, error) { return t.Time, nil }

// All episode-level work remains in the database. The result consists of one
// metadata row per card and its distinct genres, not thousands of Media objects.
func (e *EmbyService) seriesCardsSQL(ctx context.Context, scope *gorm.DB, keys []embySeriesPageKey, keyColumn, order string, latest bool) ([]map[string]any, error) {
	ids := make([]string, len(keys))
	for i := range keys {
		ids[i] = keys[i].GroupKey
	}
	part := "COALESCE(part_group_key, '') <> ''"
	if latest {
		part = "1=0"
	}
	poster := "CASE WHEN " + part + " THEN COALESCE(NULLIF(TRIM(poster_url), ''), generated_poster_url) ELSE poster_url END"
	backdrop := "CASE WHEN " + part + " THEN COALESCE(NULLIF(TRIM(backdrop_url), ''), generated_backdrop_url) ELSE backdrop_url END"
	genres := "json_extract(emby_genre_variants, '$.auto')"
	if scope.Dialector.Name() == "postgres" {
		genres = "CAST(emby_genre_variants AS jsonb) -> 'auto'"
	}
	rank := "ROW_NUMBER() OVER (PARTITION BY " + keyColumn + " ORDER BY " + order + ") AS rn"
	base := scope.Session(&gorm.Session{}).Where(keyColumn+" IN ?", ids).Select(keyColumn + ` AS group_key, id, year,
created_at, emby_premiere_date, CASE WHEN ` + part + ` THEN 1 ELSE 0 END AS multipart,
CASE WHEN ` + part + ` THEN 1 WHEN season_num < 0 THEN 1 ELSE COALESCE(season_num,0) END AS season,
COALESCE((` + poster + `),'')<>'' AS has_poster, COALESCE((` + backdrop + `),'')<>'' AS has_backdrop,
COALESCE(overview,'')<>'' AS has_overview, rating>0 AS has_rating, ` + rank)
	cte := `WITH ranked AS (?), stats AS (
SELECT group_key, COUNT(*) AS episode_count, COUNT(DISTINCT season) AS season_count,
MAX(created_at) AS added_at, MAX(NULLIF(emby_premiere_date,'')) AS premiere,
MAX(CASE WHEN year>0 THEN year END) AS production_year,
MIN(CASE WHEN has_poster THEN rn END) AS poster_rn,
MIN(CASE WHEN has_backdrop THEN rn END) AS backdrop_rn,
MIN(CASE WHEN has_overview THEN rn END) AS overview_rn,
MIN(CASE WHEN has_rating THEN rn END) AS rating_rn
FROM ranked GROUP BY group_key)
SELECT s.group_key, s.episode_count, s.season_count, s.added_at,
CASE WHEN ar.multipart=1 THEN a.emby_premiere_date ELSE s.premiere END AS release_date,
CASE WHEN ar.multipart=1 THEN a.year ELSE COALESCE(NULLIF(a.year,0),s.production_year) END AS year,
CASE WHEN ar.multipart=1 THEN a.rating ELSE r.rating END AS rating,
a.library_id, a.emby_series_name, a.title, a.part_group_title, ar.multipart, a.tm_db_id, a.bangumi_id,
CASE WHEN ar.multipart=1 THEN COALESCE(NULLIF(TRIM(p.poster_url),''),p.generated_poster_url) ELSE p.poster_url END AS poster,
CASE WHEN ar.multipart=1 THEN COALESCE(NULLIF(TRIM(b.backdrop_url),''),b.generated_backdrop_url) ELSE b.backdrop_url END AS backdrop, o.overview
FROM stats s JOIN ranked ar ON ar.group_key=s.group_key AND ar.rn=1
JOIN media a ON a.id=ar.id
LEFT JOIN ranked pr ON pr.group_key=s.group_key AND pr.rn=s.poster_rn
LEFT JOIN media p ON p.id=pr.id
LEFT JOIN ranked br ON br.group_key=s.group_key AND br.rn=s.backdrop_rn
LEFT JOIN media b ON b.id=br.id
LEFT JOIN ranked ov ON ov.group_key=s.group_key AND ov.rn=s.overview_rn
LEFT JOIN media o ON o.id=ov.id
LEFT JOIN ranked rr ON rr.group_key=s.group_key AND rr.rn=s.rating_rn
LEFT JOIN media r ON r.id=rr.id`
	var cards []struct {
		GroupKey, LibraryID, EmbySeriesName, Title, PartGroupTitle, Poster, Backdrop, Overview, ReleaseDate string
		EpisodeCount, SeasonCount, Multipart, Year, TMDbID, BangumiID                                       int
		Rating                                                                                              float32
		AddedAt                                                                                             embySQLTime
	}
	if err := e.repo.DB.WithContext(ctx).Raw(cte, base).Scan(&cards).Error; err != nil {
		return nil, err
	}
	// Preserve first-occurrence genre order and spelling across episodes. These
	// rows are deduplicated in SQL before being sent to Go.
	genreBase := scope.Session(&gorm.Session{}).Where(keyColumn+" IN ?", ids).Select(keyColumn + " AS group_key, " + genres + " AS genres, " + rank)
	expand := `SELECT r.group_key, r.rn, CAST(g.key AS INTEGER) AS ord, g.value AS name FROM genre_sets r, json_each(r.genres) g`
	if scope.Dialector.Name() == "postgres" {
		expand = `SELECT r.group_key,r.rn,g.ord,g.name FROM genre_sets r, jsonb_array_elements_text(r.genres) WITH ORDINALITY AS g(name,ord)`
	}
	genreSQL := `WITH ranked AS (?), genre_sets AS (SELECT group_key, genres, MIN(rn) AS rn FROM ranked GROUP BY group_key,genres), names AS (` + expand + `), unique_names AS (
SELECT group_key,name,rn,ord,ROW_NUMBER() OVER (PARTITION BY group_key,LOWER(TRIM(name)) ORDER BY rn,ord) AS occurrence FROM names WHERE TRIM(name)<>'')
SELECT group_key,name FROM unique_names WHERE occurrence=1 ORDER BY group_key,rn,ord`
	var names []struct{ GroupKey, Name string }
	if err := e.repo.DB.WithContext(ctx).Raw(genreSQL, genreBase).Scan(&names).Error; err != nil {
		return nil, err
	}
	byGenre := map[string][]string{}
	for _, n := range names {
		byGenre[n.GroupKey] = append(byGenre[n.GroupKey], strings.TrimSpace(n.Name))
	}
	groups := make([]embySeriesGroup, len(cards))
	ordinary := []embySeriesGroup{}
	for i, c := range cards {
		name := c.EmbySeriesName
		if c.Multipart == 1 {
			name = firstNonEmpty(c.PartGroupTitle, c.Title)
		}
		values := byGenre[c.GroupKey]
		if values == nil {
			values = []string{}
		}
		groups[i] = embySeriesGroup{ID: c.GroupKey, LibraryID: c.LibraryID, Name: name, PosterURL: c.Poster, BackdropURL: c.Backdrop, Overview: c.Overview, Rating: c.Rating, Year: c.Year, ReleaseDate: c.ReleaseDate, TMDbID: c.TMDbID, BangumiID: c.BangumiID, CreatedAt: c.AddedAt.Time, Genres: values}
		if c.Multipart == 0 {
			ordinary = append(ordinary, groups[i])
		}
	}
	if err := e.applyPersistedSeriesMetadata(ctx, ordinary); err != nil {
		return nil, err
	}
	canonical := map[string]embySeriesGroup{}
	for _, g := range ordinary {
		canonical[g.ID] = g
	}
	items := map[string]map[string]any{}
	counts := map[string]int{}
	for i, c := range cards {
		g := groups[i]
		if value, ok := canonical[g.ID]; ok {
			g = value
		}
		items[g.ID] = e.seriesCardPayload(g, c.EpisodeCount, c.SeasonCount)
		counts[g.ID] = c.EpisodeCount
	}
	out := make([]map[string]any, 0, len(keys))
	for _, key := range keys {
		if counts[key.GroupKey] != key.EpisodeCount {
			return nil, fmt.Errorf("Emby group %q changed membership during pagination", key.GroupKey)
		}
		out = append(out, items[key.GroupKey])
	}
	return out, nil
}
