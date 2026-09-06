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

// SeriesCardGroupCandidate is one representative row per persisted
// (library_id, series_key) group.  The query keeps only the narrow columns
// needed to build a card; the selected rows are hydrated separately.
type SeriesCardGroupCandidate struct {
	ID               string    `gorm:"column:id"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
	LibraryID        string    `gorm:"column:library_id"`
	SeriesID         string    `gorm:"column:series_id"`
	SeriesKey        string    `gorm:"column:series_key"`
	SeriesKeyVersion int       `gorm:"column:series_key_version"`
	Title            string    `gorm:"column:title"`
	OriginalName     string    `gorm:"column:original_name"`
	Path             string    `gorm:"column:path"`
	PosterURL        string    `gorm:"column:poster_url"`
	BackdropURL      string    `gorm:"column:backdrop_url"`
	Rating           float32   `gorm:"column:rating"`
	Year             int       `gorm:"column:year"`
	ReleaseDate      string    `gorm:"column:release_date"`
	SeasonNum        int       `gorm:"column:season_num"`
	EpisodeNum       int       `gorm:"column:episode_num"`
	ScrapeStatus     string    `gorm:"column:scrape_status"`
	TMDbID           int       `gorm:"column:tm_db_id"`
	BangumiID        int       `gorm:"column:bangumi_id"`
	DoubanID         string    `gorm:"column:douban_id"`
	TheTVDBID        string    `gorm:"column:thetvdb_id"`
	NSFW             bool      `gorm:"column:nsfw"`
	SeriesCount      int64     `gorm:"column:series_count"`
	RatingSum        float64   `gorm:"column:rating_sum"`
	RatingCount      int64     `gorm:"column:rating_count"`
}

func (c SeriesCardGroupCandidate) Media() model.Media {
	return model.Media{
		Base: model.Base{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		},
		LibraryID:        c.LibraryID,
		SeriesID:         c.SeriesID,
		SeriesKey:        c.SeriesKey,
		SeriesKeyVersion: c.SeriesKeyVersion,
		Title:            c.Title,
		OriginalName:     c.OriginalName,
		Path:             c.Path,
		PosterURL:        c.PosterURL,
		BackdropURL:      c.BackdropURL,
		Rating:           c.Rating,
		Year:             c.Year,
		ReleaseDate:      c.ReleaseDate,
		SeasonNum:        c.SeasonNum,
		EpisodeNum:       c.EpisodeNum,
		ScrapeStatus:     c.ScrapeStatus,
		TMDbID:           c.TMDbID,
		BangumiID:        c.BangumiID,
		DoubanID:         c.DoubanID,
		TheTVDBID:        c.TheTVDBID,
		NSFW:             c.NSFW,
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
	c.Path = m.Path
	c.PosterURL = m.PosterURL
	c.BackdropURL = m.BackdropURL
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
	var stale int64
	where, args := seriesGroupWhereClause("", libraryIDs, filter)
	args = append([]any{mediaSeriesKeyVersion}, args...)
	if err := r.db.WithContext(ctx).Raw("SELECT COUNT(1) FROM media WHERE deleted_at IS NULL AND (series_key_version <> ? OR series_key_version IS NULL OR series_key IS NULL OR series_key = '')"+where, args...).Scan(&stale).Error; err != nil {
		return nil, false, err
	}
	if stale > 0 {
		return nil, false, nil
	}

	grouped := r.db.WithContext(ctx).Table("media AS grouped_media").
		Select(`MIN(grouped_media.id) AS sample_id,
  grouped_media.library_id, grouped_media.series_key,
  COUNT(*) AS series_count,
  SUM(CASE WHEN grouped_media.rating > 0 THEN grouped_media.rating ELSE 0 END) AS rating_sum,
  SUM(CASE WHEN grouped_media.rating > 0 THEN 1 ELSE 0 END) AS rating_count,
  MAX(grouped_media.created_at) AS series_latest`).
		Where("grouped_media.deleted_at IS NULL AND grouped_media.series_key_version = ? AND grouped_media.series_key <> ''", mediaSeriesKeyVersion)
	grouped = applySeriesGroupScope(grouped, "grouped_media", libraryIDs, filter).
		Group("grouped_media.library_id, grouped_media.series_key")
	var aggregates []persistedSeriesGroupAggregate
	if err := grouped.Order("series_latest DESC, grouped_media.library_id DESC, grouped_media.series_key DESC").Scan(&aggregates).Error; err != nil {
		return nil, false, err
	}
	if len(aggregates) == 0 {
		return []SeriesCardGroupCandidate{}, true, nil
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
			return nil, false, err
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
	return rows, true, nil
}

// ListMediaBySeriesCardGroupsFiltered loads one narrow representative row per
// already-selected physical group. PostgreSQL uses DISTINCT ON together with
// the functional representative index; SQLite keeps the small-test fallback.
func (r *MediaRepository) ListMediaBySeriesCardGroupsFiltered(ctx context.Context, groups []SeriesCardGroupKey, filter MediaQueryFilter) ([]model.Media, error) {
	if r == nil || r.db == nil || len(groups) == 0 {
		return []model.Media{}, nil
	}
	values := make([][]any, 0, len(groups))
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
		values = append(values, []any{group.LibraryID, group.SeriesKey})
	}
	if len(values) == 0 {
		return []model.Media{}, nil
	}
	var rows []model.Media
	q := r.db.WithContext(ctx).Model(&model.Media{})
	columns := []string{
		"id", "created_at", "updated_at", "library_id", "series_id",
		"series_key", "series_key_version", "title", "original_name", "path",
		"poster_url", "backdrop_url", "rating", "year", "release_date",
		"season_num", "episode_num", "scrape_status", "tm_db_id", "bangumi_id",
		"douban_id", "thetvdb_id", "nsfw",
	}
	if r.db.Dialector.Name() == "postgres" {
		// The functional representative index makes DISTINCT ON stop at the
		// first row for each persisted physical group. This avoids loading every
		// episode into Go merely to rediscover the same representative ordering.
		q = q.Select("DISTINCT ON (library_id, series_key) " + strings.Join(columns, ", "))
	} else {
		q = q.Select(columns)
	}
	q = q.Where("deleted_at IS NULL AND series_key_version = ? AND series_key <> ''", mediaSeriesKeyVersion).
		Where("(library_id, series_key) IN ?", values)
	q = applyMediaQueryFilter(q, filter)
	order := "created_at DESC, id DESC"
	if r.db.Dialector.Name() == "postgres" {
		order = "library_id, series_key, " + persistedSeriesRepresentativeOrder
	}
	if err := q.Order(order).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
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
