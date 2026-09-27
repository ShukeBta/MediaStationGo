package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/searchspec"
)

type persistedWorkSearchMatch struct {
	SampleID    string `gorm:"column:sample_id"`
	LibraryID   string `gorm:"column:library_id"`
	SeriesKey   string `gorm:"column:series_key"`
	SearchRank  int    `gorm:"column:search_rank"`
	TotalGroups int64  `gorm:"column:total_groups"`
}

// SearchPersistedWorkGroupsPage selects persisted work identities before
// loading representative media. Multiple queries are ORed so ingest duplicate
// detection can issue one bounded request instead of serial search calls.
func (r *MediaRepository) SearchPersistedWorkGroupsPage(
	ctx context.Context,
	queries []string,
	libraryIDs []string,
	filter MediaQueryFilter,
	offset, limit int,
) ([]SeriesCardGroupCandidate, int64, bool, error) {
	if r == nil || r.db == nil {
		return nil, 0, false, nil
	}
	complete, err := r.persistedWorkSearchProjectionComplete(ctx, libraryIDs, filter)
	if err != nil || !complete {
		return nil, 0, complete, err
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 48
	}

	matchSQL, matchArgs, hasSearchTerms := workSearchPredicate(queries, "search_media")
	if len(normalizedWorkQueries(queries)) > 0 && !hasSearchTerms {
		return []SeriesCardGroupCandidate{}, 0, true, nil
	}
	grouped := r.workSearchMatchedGroupsQuery(ctx, libraryIDs, filter, matchSQL, matchArgs)
	if grouped == nil {
		return []SeriesCardGroupCandidate{}, 0, true, nil
	}
	rankSQL, rankArgs := workSearchRank(queries, "search_media")
	selectSQL := `MIN(search_media.id) AS sample_id,
  search_media.library_id, search_media.series_key, ` + rankSQL + ` AS search_rank,
  MAX(search_media.created_at) AS series_latest,
  COUNT(*) OVER() AS total_groups`
	grouped = grouped.Select(selectSQL, rankArgs...)

	var matches []persistedWorkSearchMatch
	if err := grouped.
		Order("search_rank ASC, series_latest DESC, search_media.library_id DESC, search_media.series_key DESC").
		Offset(offset).Limit(limit).Scan(&matches).Error; err != nil {
		return nil, 0, false, err
	}
	var total int64
	if len(matches) > 0 {
		total = matches[0].TotalGroups
	} else if offset > 0 {
		counted := r.workSearchMatchedGroupsQuery(ctx, libraryIDs, filter, matchSQL, matchArgs)
		if counted == nil {
			return []SeriesCardGroupCandidate{}, 0, true, nil
		}
		if err := r.db.WithContext(ctx).Table("(?) AS matched_work_groups", counted.Select("1")).Count(&total).Error; err != nil {
			return nil, 0, false, err
		}
	}
	if len(matches) == 0 {
		return []SeriesCardGroupCandidate{}, total, true, nil
	}

	keys := make([]SeriesCardGroupKey, 0, len(matches))
	for _, match := range matches {
		keys = append(keys, SeriesCardGroupKey{LibraryID: match.LibraryID, SeriesKey: match.SeriesKey})
	}
	aggregates, err := r.loadWorkSearchAggregates(ctx, keys, libraryIDs, filter)
	if err != nil {
		return nil, 0, false, err
	}
	rows, err := r.loadPersistedSeriesGroupCandidates(ctx, aggregates, filter)
	return rows, total, true, err
}

func (r *MediaRepository) workSearchMatchedGroupsQuery(
	ctx context.Context,
	libraryIDs []string,
	filter MediaQueryFilter,
	matchSQL string,
	matchArgs []any,
) *gorm.DB {
	q := r.db.WithContext(ctx).Table("media AS search_media").
		Where("search_media.deleted_at IS NULL AND search_media.series_key_version = ? AND search_media.series_key <> ''", mediaSeriesKeyVersion)
	q = applySeriesGroupScope(q, "search_media", libraryIDs, filter)
	if matchSQL != "" {
		q = q.Where(matchSQL, matchArgs...)
	}
	return q.Group("search_media.library_id, search_media.series_key")
}

func (r *MediaRepository) loadWorkSearchAggregates(
	ctx context.Context,
	keys []SeriesCardGroupKey,
	libraryIDs []string,
	filter MediaQueryFilter,
) ([]persistedSeriesGroupAggregate, error) {
	grouped := r.persistedSeriesGroupQuery(ctx, libraryIDs, filter)
	groupSQL, groupArgs := exactSeriesGroupPredicate(keys, "grouped_media")
	if groupSQL == "" {
		return []persistedSeriesGroupAggregate{}, nil
	}
	var unordered []persistedSeriesGroupAggregate
	if err := grouped.Where(groupSQL, groupArgs...).Scan(&unordered).Error; err != nil {
		return nil, err
	}
	byKey := make(map[SeriesCardGroupKey]persistedSeriesGroupAggregate, len(unordered))
	for _, aggregate := range unordered {
		byKey[SeriesCardGroupKey{LibraryID: aggregate.LibraryID, SeriesKey: aggregate.SeriesKey}] = aggregate
	}
	ordered := make([]persistedSeriesGroupAggregate, 0, len(keys))
	for _, key := range keys {
		if aggregate, ok := byKey[key]; ok {
			ordered = append(ordered, aggregate)
		}
	}
	return ordered, nil
}

func (r *MediaRepository) persistedWorkSearchProjectionComplete(ctx context.Context, libraryIDs []string, filter MediaQueryFilter) (bool, error) {
	complete, err := r.persistedSeriesKeysComplete(ctx, libraryIDs, filter)
	if err != nil || !complete {
		return complete, err
	}
	where, args := seriesGroupWhereClause("media", libraryIDs, filter)
	args = append([]any{EmbyKeyVersion}, args...)
	var incomplete int64
	query := `SELECT COUNT(1)
FROM media AS media
WHERE media.deleted_at IS NULL
  AND (COALESCE(media.season_num, 0) > 0 OR COALESCE(media.episode_num, 0) > 0 OR COALESCE(media.series_id, '') <> '')
  AND (media.emby_key_version <> ? OR media.emby_key_version IS NULL OR TRIM(COALESCE(media.emby_series_name, '')) = '')` + where
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&incomplete).Error; err != nil {
		return false, err
	}
	return incomplete == 0, nil
}

func workSearchPredicate(queries []string, alias string) (string, []any, bool) {
	document := searchspec.WorkDocumentSQL(alias)
	queryClauses := make([]string, 0, len(queries))
	args := make([]any, 0, len(queries)*2)
	for _, query := range normalizedWorkQueries(queries) {
		terms := mediaSearchTerms(query)
		termClauses := make([]string, 0, len(terms))
		for _, term := range terms {
			if year, ok := workSearchYear(term); ok {
				termClauses = append(termClauses, alias+".year = ?")
				args = append(args, year)
				continue
			}
			termClauses = append(termClauses, document+` LIKE ? ESCAPE '\'`)
			args = append(args, "%"+escapeLike(strings.ToLower(term))+"%")
		}
		if len(termClauses) > 0 {
			queryClauses = append(queryClauses, "("+strings.Join(termClauses, " AND ")+")")
		}
	}
	if len(queryClauses) == 0 {
		return "", args, false
	}
	return "(" + strings.Join(queryClauses, " OR ") + ")", args, true
}

func workSearchRank(queries []string, alias string) (string, []any) {
	normalized := normalizedWorkQueries(queries)
	if len(normalized) == 0 {
		return "0", nil
	}
	title := searchspec.WorkTitleSQL(alias)
	rankTitle := strings.ToLower(strings.TrimSpace(normalized[0]))
	return fmt.Sprintf(`MIN(CASE
  WHEN LOWER(TRIM(%s)) = ? THEN 0
  WHEN LOWER(TRIM(%s)) LIKE ? ESCAPE '\' THEN 1
  ELSE 2
END)`, title, title), []any{rankTitle, escapeLike(rankTitle) + "%"}
}

func exactSeriesGroupPredicate(keys []SeriesCardGroupKey, alias string) (string, []any) {
	if len(keys) == 0 {
		return "", nil
	}
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	clauses := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*2)
	for _, key := range keys {
		clauses = append(clauses, "("+prefix+"library_id = ? AND "+prefix+"series_key = ?)")
		args = append(args, key.LibraryID, key.SeriesKey)
	}
	return "(" + strings.Join(clauses, " OR ") + ")", args
}

func normalizedWorkQueries(queries []string) []string {
	out := make([]string, 0, len(queries))
	seen := make(map[string]struct{}, len(queries))
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		key := strings.ToLower(query)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, query)
	}
	return out
}

func workSearchYear(term string) (int, bool) {
	if len(term) != 4 {
		return 0, false
	}
	year, err := strconv.Atoi(term)
	return year, err == nil && year >= 1800 && year <= 2200
}
