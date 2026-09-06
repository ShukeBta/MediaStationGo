package repository

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const mediaSeriesKeyVersion = 1

const persistedSeriesRepresentativeOrder = `
CASE
  WHEN COALESCE(poster_url, '') = '' THEN CASE WHEN COALESCE(backdrop_url, '') <> '' THEN 5 ELSE 0 END
  WHEN LOWER(poster_url) ~ '(poster|folder|cover|movie|show|pl)([._-]|\.[a-z0-9]+$|$)' THEN 40
  WHEN LOWER(poster_url) ~ '(actor|actress|cast|avatar|sample|screenshot|screen|still|scene|fanart|backdrop|background|landscape|banner|logo|disc)' THEN 10
  WHEN POSITION('thumb' IN LOWER(poster_url)) > 0 THEN 20
  ELSE 30
END DESC,
CASE WHEN season_num > 0 OR episode_num > 0 THEN season_num * 10000 + episode_num ELSE 0 END,
created_at DESC, id DESC`

// SeriesCardGroupCandidate carries one identity sample plus aggregate values
// per persisted (library_id, series_key) group. Only groups selected by SQL
// pagination/Top-N are resolved to their representative card projection.
type SeriesCardGroupCandidate struct {
	ID                   string    `gorm:"column:id"`
	CreatedAt            time.Time `gorm:"column:created_at"`
	UpdatedAt            time.Time `gorm:"column:updated_at"`
	LibraryID            string    `gorm:"column:library_id"`
	SeriesID             string    `gorm:"column:series_id"`
	SeriesKey            string    `gorm:"column:series_key"`
	SeriesKeyVersion     int       `gorm:"column:series_key_version"`
	Title                string    `gorm:"column:title"`
	OriginalName         string    `gorm:"column:original_name"`
	EpisodeTitle         string    `gorm:"column:episode_title"`
	Path                 string    `gorm:"column:path"`
	PosterURL            string    `gorm:"column:poster_url"`
	BackdropURL          string    `gorm:"column:backdrop_url"`
	GeneratedPosterURL   string    `gorm:"column:generated_poster_url"`
	GeneratedBackdropURL string    `gorm:"column:generated_backdrop_url"`
	Overview             string    `gorm:"column:overview"`
	Rating               float32   `gorm:"column:rating"`
	Year                 int       `gorm:"column:year"`
	ReleaseDate          string    `gorm:"column:release_date"`
	SeasonNum            int       `gorm:"column:season_num"`
	EpisodeNum           int       `gorm:"column:episode_num"`
	ScrapeStatus         string    `gorm:"column:scrape_status"`
	TMDbID               int       `gorm:"column:tm_db_id"`
	BangumiID            int       `gorm:"column:bangumi_id"`
	DoubanID             string    `gorm:"column:douban_id"`
	TheTVDBID            string    `gorm:"column:thetvdb_id"`
	Languages            string    `gorm:"column:languages"`
	Countries            string    `gorm:"column:countries"`
	Genres               string    `gorm:"column:genres"`
	Actors               string    `gorm:"column:actors"`
	Width                int       `gorm:"column:width"`
	Height               int       `gorm:"column:height"`
	VideoCodec           string    `gorm:"column:video_codec"`
	NSFW                 bool      `gorm:"column:nsfw"`
	SeriesCount          int64     `gorm:"column:series_count"`
	RatingSum            float64   `gorm:"column:rating_sum"`
	RatingCount          int64     `gorm:"column:rating_count"`
}

func (c SeriesCardGroupCandidate) Media() model.Media {
	return model.Media{
		Base: model.Base{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		},
		LibraryID:            c.LibraryID,
		SeriesID:             c.SeriesID,
		SeriesKey:            c.SeriesKey,
		SeriesKeyVersion:     c.SeriesKeyVersion,
		Title:                c.Title,
		OriginalName:         c.OriginalName,
		EpisodeTitle:         c.EpisodeTitle,
		Path:                 c.Path,
		PosterURL:            c.PosterURL,
		BackdropURL:          c.BackdropURL,
		GeneratedPosterURL:   c.GeneratedPosterURL,
		GeneratedBackdropURL: c.GeneratedBackdropURL,
		Overview:             c.Overview,
		Rating:               c.Rating,
		Year:                 c.Year,
		ReleaseDate:          c.ReleaseDate,
		SeasonNum:            c.SeasonNum,
		EpisodeNum:           c.EpisodeNum,
		ScrapeStatus:         c.ScrapeStatus,
		TMDbID:               c.TMDbID,
		BangumiID:            c.BangumiID,
		DoubanID:             c.DoubanID,
		TheTVDBID:            c.TheTVDBID,
		Languages:            c.Languages,
		Countries:            c.Countries,
		Genres:               c.Genres,
		Actors:               c.Actors,
		Width:                c.Width,
		Height:               c.Height,
		VideoCodec:           c.VideoCodec,
		NSFW:                 c.NSFW,
	}
}

// WithMedia keeps the aggregate values of a persisted group while replacing
// its lightweight identity sample with the exact representative selected by
// the service's authoritative artwork rules.
func (c SeriesCardGroupCandidate) WithMedia(m model.Media) SeriesCardGroupCandidate {
	c.ID = m.ID
	c.CreatedAt = m.CreatedAt
	c.UpdatedAt = m.UpdatedAt
	c.LibraryID = m.LibraryID
	c.SeriesID = m.SeriesID
	c.SeriesKey = m.SeriesKey
	c.SeriesKeyVersion = m.SeriesKeyVersion
	c.Title = m.Title
	c.OriginalName = m.OriginalName
	c.EpisodeTitle = m.EpisodeTitle
	c.Path = m.Path
	c.PosterURL = m.PosterURL
	c.BackdropURL = m.BackdropURL
	c.GeneratedPosterURL = m.GeneratedPosterURL
	c.GeneratedBackdropURL = m.GeneratedBackdropURL
	c.Overview = m.Overview
	c.Rating = m.Rating
	c.Year = m.Year
	c.ReleaseDate = m.ReleaseDate
	c.SeasonNum = m.SeasonNum
	c.EpisodeNum = m.EpisodeNum
	c.ScrapeStatus = m.ScrapeStatus
	c.TMDbID = m.TMDbID
	c.BangumiID = m.BangumiID
	c.DoubanID = m.DoubanID
	c.TheTVDBID = m.TheTVDBID
	c.Languages = m.Languages
	c.Countries = m.Countries
	c.Genres = m.Genres
	c.Actors = m.Actors
	c.Width = m.Width
	c.Height = m.Height
	c.VideoCodec = m.VideoCodec
	c.NSFW = m.NSFW
	return c
}

type SeriesCardGroupKey struct {
	LibraryID string
	SeriesKey string
}

type persistedSeriesGroupAggregate struct {
	SampleID    string  `gorm:"column:sample_id"`
	LibraryID   string  `gorm:"column:library_id"`
	SeriesKey   string  `gorm:"column:series_key"`
	SeriesCount int64   `gorm:"column:series_count"`
	RatingSum   float64 `gorm:"column:rating_sum"`
	RatingCount int64   `gorm:"column:rating_count"`
	TotalGroups int64   `gorm:"column:total_groups"`
}

// ListPersistedSeriesCardGroups returns one lightweight identity sample plus
// aggregate values per persisted series group. It deliberately does not rank
// artwork across every episode: callers first select the requested logical
// cards, then load only those groups through ListMediaBySeriesCardGroupsFiltered
// and apply the authoritative Go representative rule.
//
// complete is false while any active row in scope is missing the current key
// version.
func (r *MediaRepository) ListPersistedSeriesCardGroups(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) ([]SeriesCardGroupCandidate, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}
	complete, err := r.persistedSeriesKeysComplete(ctx, libraryIDs, filter)
	if err != nil || !complete {
		return nil, complete, err
	}

	grouped := r.persistedSeriesGroupQuery(ctx, libraryIDs, filter)
	var aggregates []persistedSeriesGroupAggregate
	if err := grouped.Order("series_latest DESC, grouped_media.library_id DESC, grouped_media.series_key DESC").Scan(&aggregates).Error; err != nil {
		return nil, false, err
	}
	rows, err := r.loadPersistedSeriesGroupCandidates(ctx, aggregates, filter)
	return rows, true, err
}

// ListPersistedSeriesCardGroupsPage lets PostgreSQL apply pagination to the
// final physical series groups. Callers must use it only when service-side
// display-library analysis proves that physical and public groups are 1:1.
func (r *MediaRepository) ListPersistedSeriesCardGroupsPage(ctx context.Context, libraryIDs []string, filter MediaQueryFilter, offset, limit int) ([]SeriesCardGroupCandidate, int64, bool, error) {
	if r == nil || r.db == nil {
		return nil, 0, false, nil
	}
	complete, err := r.persistedSeriesKeysComplete(ctx, libraryIDs, filter)
	if err != nil || !complete {
		return nil, 0, complete, err
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 48
	}

	grouped := r.persistedSeriesGroupQuery(ctx, libraryIDs, filter).
		Select(persistedSeriesGroupSelect + ", COUNT(*) OVER() AS total_groups")
	var aggregates []persistedSeriesGroupAggregate
	if err := grouped.Order("series_latest DESC, grouped_media.library_id DESC, grouped_media.series_key DESC").Offset(offset).Limit(limit).Scan(&aggregates).Error; err != nil {
		return nil, 0, false, err
	}
	var total int64
	if len(aggregates) > 0 {
		total = aggregates[0].TotalGroups
	} else if offset > 0 {
		counted, countErr := r.countPersistedSeriesGroups(ctx, libraryIDs, filter)
		if countErr != nil {
			return nil, 0, false, countErr
		}
		total = counted
	}
	rows, err := r.loadPersistedSeriesGroupCandidates(ctx, aggregates, filter)
	return rows, total, true, err
}

// ListRecentPersistedSeriesCardGroups returns only the requested Top-N groups.
// As with the paged query, callers must first prove physical/public grouping is
// 1:1 for the current library configuration.
func (r *MediaRepository) ListRecentPersistedSeriesCardGroups(ctx context.Context, filter MediaQueryFilter, limit int) ([]SeriesCardGroupCandidate, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}
	complete, err := r.persistedSeriesKeysComplete(ctx, nil, filter)
	if err != nil || !complete {
		return nil, complete, err
	}
	if limit <= 0 {
		limit = 24
	}
	var aggregates []persistedSeriesGroupAggregate
	if err := r.persistedSeriesGroupQuery(ctx, nil, filter).
		Order("series_latest DESC, grouped_media.library_id DESC, grouped_media.series_key DESC").
		Limit(limit).Scan(&aggregates).Error; err != nil {
		return nil, false, err
	}
	rows, err := r.loadPersistedSeriesGroupCandidates(ctx, aggregates, filter)
	return rows, true, err
}

// ListFeaturedPersistedSeriesCardGroups performs rating qualification and
// Top-N selection in SQL before representative rows are loaded.
func (r *MediaRepository) ListFeaturedPersistedSeriesCardGroups(ctx context.Context, filter MediaQueryFilter, minimumRating float64, limit int) ([]SeriesCardGroupCandidate, bool, error) {
	if r == nil || r.db == nil {
		return nil, false, nil
	}
	complete, err := r.persistedSeriesKeysComplete(ctx, nil, filter)
	if err != nil || !complete {
		return nil, complete, err
	}
	if limit <= 0 {
		limit = 20
	}
	ratingSum := "SUM(CASE WHEN grouped_media.rating > 0 THEN grouped_media.rating ELSE 0 END)"
	ratingCount := "SUM(CASE WHEN grouped_media.rating > 0 THEN 1 ELSE 0 END)"
	var aggregates []persistedSeriesGroupAggregate
	if err := r.persistedSeriesGroupQuery(ctx, nil, filter).
		Having(ratingCount+" > 0 AND "+ratingSum+" >= ? * "+ratingCount, minimumRating).
		Order(ratingSum + " / NULLIF(" + ratingCount + ", 0) DESC, grouped_media.library_id DESC, grouped_media.series_key DESC").
		Limit(limit).Scan(&aggregates).Error; err != nil {
		return nil, false, err
	}
	rows, err := r.loadPersistedSeriesGroupCandidates(ctx, aggregates, filter)
	return rows, true, err
}

const persistedSeriesGroupSelect = `MIN(grouped_media.id) AS sample_id,
  grouped_media.library_id, grouped_media.series_key,
  COUNT(*) AS series_count,
  SUM(CASE WHEN grouped_media.rating > 0 THEN grouped_media.rating ELSE 0 END) AS rating_sum,
  SUM(CASE WHEN grouped_media.rating > 0 THEN 1 ELSE 0 END) AS rating_count,
  MAX(grouped_media.created_at) AS series_latest`

func (r *MediaRepository) persistedSeriesGroupQuery(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) *gorm.DB {
	grouped := r.db.WithContext(ctx).Table("media AS grouped_media").
		Select(persistedSeriesGroupSelect).
		Where("grouped_media.deleted_at IS NULL AND grouped_media.series_key_version = ? AND grouped_media.series_key <> ''", mediaSeriesKeyVersion)
	return applySeriesGroupScope(grouped, "grouped_media", libraryIDs, filter).
		Group("grouped_media.library_id, grouped_media.series_key")
}

func (r *MediaRepository) persistedSeriesKeysComplete(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) (bool, error) {
	var stale int64
	where, args := seriesGroupWhereClause("", libraryIDs, filter)
	args = append([]any{mediaSeriesKeyVersion}, args...)
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(1) FROM media WHERE deleted_at IS NULL AND (series_key_version <> ? OR series_key_version IS NULL OR series_key IS NULL OR series_key = '')"+where, args...).Scan(&stale).Error; err != nil {
		return false, err
	}
	return stale == 0, nil
}

func (r *MediaRepository) countPersistedSeriesGroups(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) (int64, error) {
	grouped := r.persistedSeriesGroupQuery(ctx, libraryIDs, filter).Select("1")
	var total int64
	err := r.db.WithContext(ctx).Table("(?) AS persisted_groups", grouped).Count(&total).Error
	return total, err
}

func (r *MediaRepository) loadPersistedSeriesGroupCandidates(ctx context.Context, aggregates []persistedSeriesGroupAggregate, filter MediaQueryFilter) ([]SeriesCardGroupCandidate, error) {
	if len(aggregates) == 0 {
		return []SeriesCardGroupCandidate{}, nil
	}

	// Fetch only the narrow identity columns for the MIN(id) samples. Keeping
	// this as a separate indexed lookup avoids making PostgreSQL rescan the
	// entire wide media table for a join against the small aggregate result.
	ids := make([]string, 0, len(aggregates))
	for _, aggregate := range aggregates {
		if strings.TrimSpace(aggregate.SampleID) != "" {
			ids = append(ids, aggregate.SampleID)
		}
	}
	var samples []SeriesCardGroupCandidate
	if len(ids) > 0 {
		sampleQuery := r.db.WithContext(ctx).Model(&model.Media{}).
			Select(`id, created_at, updated_at, library_id, series_id, series_key,
  series_key_version, title, original_name, path, season_num, episode_num,
  scrape_status, tm_db_id, bangumi_id, douban_id, thetvdb_id, nsfw`).
			Where("deleted_at IS NULL AND id IN ?", ids)
		sampleQuery = applyMediaQueryFilter(sampleQuery, filter)
		if err := sampleQuery.Scan(&samples).Error; err != nil {
			return nil, err
		}
	}
	byID := make(map[string]SeriesCardGroupCandidate, len(samples))
	for _, sample := range samples {
		byID[sample.ID] = sample
	}
	rows := make([]SeriesCardGroupCandidate, 0, len(aggregates))
	for _, aggregate := range aggregates {
		sample, ok := byID[aggregate.SampleID]
		if !ok {
			// A concurrent delete can remove the identity sample between the
			// aggregate and indexed lookup. Omit that vanished group; the next
			// request will observe a fresh aggregate instead of returning a
			// fabricated card or failing the whole library listing.
			continue
		}
		sample.SeriesCount = aggregate.SeriesCount
		sample.RatingSum = aggregate.RatingSum
		sample.RatingCount = aggregate.RatingCount
		rows = append(rows, sample)
	}
	return rows, nil
}

// ListMediaBySeriesCardGroupsFiltered loads one card-sized representative row
// per already-selected physical group. PostgreSQL probes the functional index
// once per group; SQLite keeps the small-test fallback.
func (r *MediaRepository) ListMediaBySeriesCardGroupsFiltered(ctx context.Context, groups []SeriesCardGroupKey, filter MediaQueryFilter) ([]model.Media, error) {
	if r == nil || r.db == nil || len(groups) == 0 {
		return []model.Media{}, nil
	}
	uniqueGroups := make([]SeriesCardGroupKey, 0, len(groups))
	seen := make(map[SeriesCardGroupKey]struct{}, len(groups))
	for _, group := range groups {
		group.LibraryID = strings.TrimSpace(group.LibraryID)
		group.SeriesKey = strings.TrimSpace(group.SeriesKey)
		if group.LibraryID == "" || group.SeriesKey == "" {
			continue
		}
		if _, exists := seen[group]; exists {
			continue
		}
		seen[group] = struct{}{}
		uniqueGroups = append(uniqueGroups, group)
	}
	if len(uniqueGroups) == 0 {
		return []model.Media{}, nil
	}
	var rows []model.Media
	columns := []string{
		"id", "created_at", "updated_at", "library_id", "series_id",
		"series_key", "series_key_version", "title", "original_name", "path",
		"episode_title", "poster_url", "backdrop_url", "generated_poster_url",
		"generated_backdrop_url", "overview", "rating", "year", "release_date",
		"season_num", "episode_num", "scrape_status", "tm_db_id", "bangumi_id",
		"douban_id", "thetvdb_id", "languages", "countries", "genres", "actors",
		"width", "height", "video_codec", "nsfw",
	}
	if r.db.Dialector.Name() == "postgres" {
		// Keep each lateral probe index-only/narrow. Fetching artwork and path
		// columns inside the probe turns one batch into many random heap reads;
		// the selected IDs can be loaded much more cheaply in one second query.
		query, args := postgresSeriesRepresentativeIDsQuery(uniqueGroups, filter)
		var representativeIDs []string
		if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&representativeIDs).Error; err != nil {
			return nil, err
		}
		if len(representativeIDs) == 0 {
			return []model.Media{}, nil
		}
		q := r.db.WithContext(ctx).Model(&model.Media{}).Select(columns).
			Where("deleted_at IS NULL AND series_key_version = ? AND series_key <> '' AND id IN ?", mediaSeriesKeyVersion, representativeIDs)
		q = applyMediaQueryFilter(q, filter)
		if err := q.Find(&rows).Error; err != nil {
			return nil, err
		}
		return rows, nil
	}

	values := make([][]any, 0, len(uniqueGroups))
	for _, group := range uniqueGroups {
		values = append(values, []any{group.LibraryID, group.SeriesKey})
	}
	q := r.db.WithContext(ctx).Model(&model.Media{}).Select(columns)
	q = q.Where("deleted_at IS NULL AND series_key_version = ? AND series_key <> ''", mediaSeriesKeyVersion).
		Where("(library_id, series_key) IN ?", values)
	q = applyMediaQueryFilter(q, filter)
	if err := q.Order("created_at DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func postgresSeriesRepresentativeIDsQuery(groups []SeriesCardGroupKey, filter MediaQueryFilter) (string, []any) {
	valueRows := make([]string, len(groups))
	args := make([]any, 0, len(groups)*2+1+len(filter.HiddenLibraryIDs)+len(filter.AllowedLibraryIDs))
	for i, group := range groups {
		valueRows[i] = "(?, ?)"
		args = append(args, group.LibraryID, group.SeriesKey)
	}

	conditions := []string{
		"representative.deleted_at IS NULL",
		"representative.series_key_version = ?",
		"representative.series_key <> ''",
		"representative.library_id = selected_groups.library_id",
		"representative.series_key = selected_groups.series_key",
	}
	args = append(args, mediaSeriesKeyVersion)
	if !filter.IncludeNSFW {
		conditions = append(conditions, "representative.nsfw = ?")
		args = append(args, false)
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		conditions = append(conditions, "representative.library_id NOT IN ("+strings.TrimSuffix(strings.Repeat("?, ", len(filter.HiddenLibraryIDs)), ", ")+")")
		for _, id := range filter.HiddenLibraryIDs {
			args = append(args, id)
		}
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		conditions = append(conditions, "representative.library_id IN ("+strings.TrimSuffix(strings.Repeat("?, ", len(filter.AllowedLibraryIDs)), ", ")+")")
		for _, id := range filter.AllowedLibraryIDs {
			args = append(args, id)
		}
	}

	query := `SELECT representative.id
FROM (VALUES ` + strings.Join(valueRows, ", ") + `) AS selected_groups(library_id, series_key)
CROSS JOIN LATERAL (
  SELECT representative.id
  FROM media AS representative
  WHERE ` + strings.Join(conditions, " AND ") + `
  ORDER BY ` + persistedSeriesRepresentativeOrder + `
  LIMIT 1
) AS representative`
	return query, args
}

func applySeriesGroupScope(q *gorm.DB, alias string, libraryIDs []string, filter MediaQueryFilter) *gorm.DB {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	if len(libraryIDs) == 1 {
		q = q.Where(prefix+"library_id = ?", libraryIDs[0])
	} else if len(libraryIDs) > 1 {
		q = q.Where(prefix+"library_id IN ?", libraryIDs)
	}
	if !filter.IncludeNSFW {
		q = q.Where(prefix+"nsfw = ?", false)
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		q = q.Where(prefix+"library_id NOT IN ?", filter.HiddenLibraryIDs)
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		q = q.Where(prefix+"library_id IN ?", filter.AllowedLibraryIDs)
	}
	return q
}

func seriesGroupWhereClause(alias string, libraryIDs []string, filter MediaQueryFilter) (string, []any) {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	clauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	if len(libraryIDs) == 1 {
		clauses = append(clauses, prefix+"library_id = ?")
		args = append(args, libraryIDs[0])
	} else if len(libraryIDs) > 1 {
		clauses = append(clauses, prefix+"library_id IN ?")
		args = append(args, libraryIDs)
	}
	if !filter.IncludeNSFW {
		clauses = append(clauses, prefix+"nsfw = ?")
		args = append(args, false)
	}
	if len(filter.HiddenLibraryIDs) > 0 {
		clauses = append(clauses, prefix+"library_id NOT IN ?")
		args = append(args, filter.HiddenLibraryIDs)
	}
	if len(filter.AllowedLibraryIDs) > 0 {
		clauses = append(clauses, prefix+"library_id IN ?")
		args = append(args, filter.AllowedLibraryIDs)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

// ListMediaBySeriesKeysFiltered fetches full episode rows after the indexed
// group lookup has identified the matching persisted keys.
func (r *MediaRepository) ListMediaBySeriesKeysFiltered(ctx context.Context, libraryIDs, seriesKeys []string, filter MediaQueryFilter) ([]model.Media, error) {
	if r == nil || r.db == nil || len(seriesKeys) == 0 {
		return []model.Media{}, nil
	}
	q := r.db.WithContext(ctx).Model(&model.Media{}).
		Where("series_key_version = ? AND series_key IN ?", mediaSeriesKeyVersion, seriesKeys)
	if len(libraryIDs) == 1 {
		q = q.Where("library_id = ?", libraryIDs[0])
	} else if len(libraryIDs) > 1 {
		q = q.Where("library_id IN ?", libraryIDs)
	}
	q = applyMediaQueryFilter(q, filter)
	var rows []model.Media
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// SeriesKeysComplete reports whether every active row in the requested scope
// has a current persisted key. It is kept separate from the group query so
// episode detail requests can fast-path directly to the composite index.
func (r *MediaRepository) SeriesKeysComplete(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) (bool, error) {
	if r == nil || r.db == nil || len(libraryIDs) == 0 {
		return true, nil
	}
	var stale int64
	where, args := seriesGroupWhereClause("", libraryIDs, filter)
	args = append([]any{mediaSeriesKeyVersion}, args...)
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(1) FROM media WHERE deleted_at IS NULL AND (series_key_version <> ? OR series_key_version IS NULL OR series_key IS NULL OR series_key = '')"+where, args...).Scan(&stale).Error; err != nil {
		return false, err
	}
	return stale == 0, nil
}
