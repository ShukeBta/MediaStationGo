package repository

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const mediaSeriesKeyVersion = 1

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

// ListPersistedSeriesCardGroups returns one lightweight representative per
// persisted series group. complete is false while any active row in scope is
// missing the current key version; callers should use the legacy grouping
// path until the background backfill catches up.
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

	q := r.db.WithContext(ctx).Table("media AS m").
		Select(`m.id, m.created_at, m.updated_at, m.library_id, m.series_id,
  m.series_key, m.series_key_version, m.title, m.original_name, m.path,
  m.poster_url, m.backdrop_url, m.rating, m.year, m.release_date,
  m.season_num, m.episode_num, m.scrape_status, m.tm_db_id, m.bangumi_id,
  m.douban_id, m.thetvdb_id, m.nsfw,
  COUNT(*) OVER (PARTITION BY m.library_id, m.series_key) AS series_count,
  SUM(CASE WHEN m.rating > 0 THEN m.rating ELSE 0 END)
    OVER (PARTITION BY m.library_id, m.series_key) AS rating_sum,
  SUM(CASE WHEN m.rating > 0 THEN 1 ELSE 0 END)
    OVER (PARTITION BY m.library_id, m.series_key) AS rating_count,
  MAX(m.created_at) OVER (PARTITION BY m.library_id, m.series_key) AS series_latest,
  ROW_NUMBER() OVER (
    PARTITION BY m.library_id, m.series_key
    ORDER BY CASE
      WHEN COALESCE(m.poster_url, '') = '' THEN
        CASE WHEN COALESCE(m.backdrop_url, '') <> '' THEN 5 ELSE 0 END
      WHEN LOWER(m.poster_url) LIKE '%poster.%'
        OR LOWER(m.poster_url) LIKE '%poster-%'
        OR LOWER(m.poster_url) LIKE '%poster'
        OR LOWER(m.poster_url) LIKE '%folder.%'
        OR LOWER(m.poster_url) LIKE '%folder-%'
        OR LOWER(m.poster_url) LIKE '%folder'
        OR LOWER(m.poster_url) LIKE '%cover.%'
        OR LOWER(m.poster_url) LIKE '%cover-%'
        OR LOWER(m.poster_url) LIKE '%cover'
        OR LOWER(m.poster_url) LIKE '%movie.%'
        OR LOWER(m.poster_url) LIKE '%movie-%'
        OR LOWER(m.poster_url) LIKE '%movie'
        OR LOWER(m.poster_url) LIKE '%show.%'
        OR LOWER(m.poster_url) LIKE '%show-%'
        OR LOWER(m.poster_url) LIKE '%show'
        OR LOWER(m.poster_url) LIKE '%pl.%'
        OR LOWER(m.poster_url) LIKE '%pl-%'
        OR LOWER(m.poster_url) LIKE '%pl' THEN 40
      WHEN LOWER(m.poster_url) LIKE '%actor%'
        OR LOWER(m.poster_url) LIKE '%actress%'
        OR LOWER(m.poster_url) LIKE '%cast%'
        OR LOWER(m.poster_url) LIKE '%avatar%'
        OR LOWER(m.poster_url) LIKE '%sample%'
        OR LOWER(m.poster_url) LIKE '%screenshot%'
        OR LOWER(m.poster_url) LIKE '%screen%'
        OR LOWER(m.poster_url) LIKE '%still%'
        OR LOWER(m.poster_url) LIKE '%scene%'
        OR LOWER(m.poster_url) LIKE '%fanart%'
        OR LOWER(m.poster_url) LIKE '%backdrop%'
        OR LOWER(m.poster_url) LIKE '%background%'
        OR LOWER(m.poster_url) LIKE '%landscape%'
        OR LOWER(m.poster_url) LIKE '%banner%'
        OR LOWER(m.poster_url) LIKE '%logo%'
        OR LOWER(m.poster_url) LIKE '%disc%' THEN 10
      WHEN LOWER(m.poster_url) LIKE '%thumb%' THEN 20
      ELSE 30 END DESC,
      CASE WHEN m.season_num > 0 OR m.episode_num > 0
        THEN m.season_num * 10000 + m.episode_num ELSE 0 END,
      m.created_at DESC, m.id DESC
  ) AS representative_rank`).
		Where("m.deleted_at IS NULL AND m.series_key_version = ? AND m.series_key <> ''", mediaSeriesKeyVersion)
	q = applySeriesGroupScope(q, "m", libraryIDs, filter)
	q = r.db.WithContext(ctx).Table("(?) AS ranked", q).
		Select(`id, created_at, updated_at, library_id, series_id,
  series_key, series_key_version, title, original_name, path,
  poster_url, backdrop_url, rating, year, release_date, season_num,
  episode_num, scrape_status, tm_db_id, bangumi_id, douban_id,
  thetvdb_id, nsfw, series_count, rating_sum, rating_count`).
		Where("representative_rank = 1").
		Order("series_latest DESC, id DESC")
	var rows []SeriesCardGroupCandidate
	if err := q.Scan(&rows).Error; err != nil {
		return nil, false, err
	}
	return rows, true, nil
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
