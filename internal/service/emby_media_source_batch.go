package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

type embyMediaVersionSiblingsContextKey struct{}

const embyMediaVersionLookupBatchSize = 100

type embyMediaVersionMatch struct {
	libraryIDs []string
	librarySet map[string]struct{}
	seasonNum  int
	episodeNum int
	kind       string
	value      string
	numericID  int
	year       int
}

func (e *EmbyService) withEmbyMediaVersionSiblings(ctx context.Context, rows []model.Media) (context.Context, error) {
	if e == nil || e.repo == nil || e.repo.DB == nil || len(rows) == 0 {
		return ctx, nil
	}
	cache := make(map[string][]model.Media, len(rows))
	matches := make(map[string]embyMediaVersionMatch)
	matchOrder := make([]string, 0, len(rows))
	matchKeyByMediaID := make(map[string]string, len(rows))
	for i := range rows {
		match, query := e.mediaVersionMatch(ctx, &rows[i])
		if !query {
			cache[rows[i].ID] = []model.Media{rows[i]}
			continue
		}
		key := match.key()
		matchKeyByMediaID[rows[i].ID] = key
		if _, exists := matches[key]; !exists {
			matches[key] = match
			matchOrder = append(matchOrder, key)
		}
	}
	if len(matchOrder) == 0 {
		return context.WithValue(ctx, embyMediaVersionSiblingsContextKey{}, cache), nil
	}

	siblingsByMatchKey := make(map[string][]model.Media, len(matchOrder))
	for start := 0; start < len(matchOrder); start += embyMediaVersionLookupBatchSize {
		end := start + embyMediaVersionLookupBatchSize
		if end > len(matchOrder) {
			end = len(matchOrder)
		}
		var query *gorm.DB
		for _, key := range matchOrder[start:end] {
			condition, args := matches[key].query()
			if query == nil {
				query = e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where(condition, args...)
			} else {
				query = query.Or(condition, args...)
			}
		}
		var batch []model.Media
		if err := query.Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, key := range matchOrder[start:end] {
			match := matches[key]
			for i := range batch {
				if match.matches(batch[i]) {
					siblingsByMatchKey[key] = append(siblingsByMatchKey[key], batch[i])
				}
			}
		}
	}
	for i := range rows {
		key, queryNeeded := matchKeyByMediaID[rows[i].ID]
		if !queryNeeded {
			continue
		}
		siblings := append([]model.Media(nil), siblingsByMatchKey[key]...)
		cache[rows[i].ID] = e.sortedMediaVersionSiblings(&rows[i], siblings)
	}
	return context.WithValue(ctx, embyMediaVersionSiblingsContextKey{}, cache), nil
}

func (e *EmbyService) mediaVersionMatch(ctx context.Context, m *model.Media) (embyMediaVersionMatch, bool) {
	if m == nil || strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.PartGroupKey) != "" {
		return embyMediaVersionMatch{}, false
	}
	libraryIDs := append([]string(nil), e.mergedLibraryIDs(ctx, m.LibraryID)...)
	if len(libraryIDs) == 0 {
		libraryIDs = []string{m.LibraryID}
	}
	sort.Strings(libraryIDs)
	match := embyMediaVersionMatch{
		libraryIDs: libraryIDs,
		librarySet: make(map[string]struct{}, len(libraryIDs)),
		seasonNum:  m.SeasonNum,
		episodeNum: m.EpisodeNum,
	}
	for _, id := range libraryIDs {
		match.librarySet[id] = struct{}{}
	}
	if key := strings.TrimSpace(m.VersionGroupKey); key != "" {
		match.kind = "version"
		match.value = m.VersionGroupKey
		return match, true
	}
	if m.TitleCleanupVersion >= mediaTitleExplicitGroupingVersion {
		return embyMediaVersionMatch{}, false
	}
	if m.TMDbID > 0 {
		match.kind = "tmdb"
		match.numericID = m.TMDbID
		return match, true
	}
	if m.BangumiID > 0 {
		match.kind = "bangumi"
		match.numericID = m.BangumiID
		return match, true
	}
	title := strings.TrimSpace(m.Title)
	if title == "" {
		title = strings.TrimSpace(m.OriginalName)
	}
	if title == "" {
		return embyMediaVersionMatch{}, false
	}
	match.kind = "title"
	match.value = strings.ToLower(title)
	match.year = m.Year
	return match, true
}

func (m embyMediaVersionMatch) key() string {
	libraries := strings.Join(m.libraryIDs, ",")
	return fmt.Sprintf("%d:%s|s:%d|e:%d|%s:%d:%s:%d|y:%d", len(libraries), libraries, m.seasonNum, m.episodeNum, m.kind, len(m.value), m.value, m.numericID, m.year)
}

func (m embyMediaVersionMatch) query() (string, []any) {
	base := "library_id IN ? AND season_num = ? AND episode_num = ?"
	args := []any{m.libraryIDs, m.seasonNum, m.episodeNum}
	switch m.kind {
	case "version":
		return base + " AND version_group_key = ?", append(args, m.value)
	case "tmdb":
		return base + " AND tm_db_id = ?", append(args, m.numericID)
	case "bangumi":
		return base + " AND bangumi_id = ?", append(args, m.numericID)
	default:
		condition := base + " AND LOWER(title) = ?"
		args = append(args, m.value)
		if m.year > 0 {
			condition += " AND year = ?"
			args = append(args, m.year)
		}
		return condition, args
	}
}

func (m embyMediaVersionMatch) matches(row model.Media) bool {
	if _, ok := m.librarySet[row.LibraryID]; !ok || row.SeasonNum != m.seasonNum || row.EpisodeNum != m.episodeNum {
		return false
	}
	switch m.kind {
	case "version":
		return row.VersionGroupKey == m.value
	case "tmdb":
		return row.TMDbID == m.numericID
	case "bangumi":
		return row.BangumiID == m.numericID
	default:
		return strings.ToLower(row.Title) == m.value && (m.year <= 0 || row.Year == m.year)
	}
}
