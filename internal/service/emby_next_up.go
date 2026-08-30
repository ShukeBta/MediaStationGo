package service

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

// embyNextUpDefaultLimit matches the default page size Emby servers use for
// /Shows/NextUp when a client omits Limit.
const embyNextUpDefaultLimit = 20

// embyNextUpQueryBatch keeps each history lookup below conservative SQLite
// host-parameter limits; production PostgreSQL gets the same batching.
const embyNextUpQueryBatch = 500

// NextUpParams carries the parsed /Shows/NextUp query.
type NextUpParams struct {
	UserID     string
	SeriesIDs  []string
	SeasonID   string
	StartIndex int
	Limit      int
}

type embyNextUpCandidate struct {
	episode  model.Media
	anchor   time.Time
	anchored bool
}

type embyNextUpSeriesHit struct {
	group    embySeriesGroup
	candidate embyNextUpCandidate
}

// NextUpItems resolves the Emby "Next Up" query: the episode a client should
// offer to continue for each requested series. Popcorn and other third-party
// clients call this right after loading a series page to decide which episode
// counts as "上次看到"; an empty envelope makes them fall back to episode 1
// even when resume positions exist.
func (e *EmbyService) NextUpItems(ctx context.Context, p NextUpParams) (map[string]any, error) {
	if p.StartIndex < 0 {
		p.StartIndex = 0
	}
	if p.Limit <= 0 {
		p.Limit = embyNextUpDefaultLimit
	} else if p.Limit > MaxEmbyItemsPageSize {
		p.Limit = MaxEmbyItemsPageSize
	}
	if strings.TrimSpace(p.UserID) == "" {
		return map[string]any{"Items": []map[string]any{}, "TotalRecordCount": 0, "StartIndex": p.StartIndex}, nil
	}

	hits := make([]embyNextUpSeriesHit, 0, len(p.SeriesIDs))
	switch {
	case p.SeasonID != "":
		season, ok, err := e.findSeasonGroup(ctx, p.SeasonID, p.UserID)
		if err != nil {
			return nil, err
		}
		if ok {
			hist, err := e.historyByMediaIDs(ctx, p.UserID, episodeMediaIDs(season.Episodes))
			if err != nil {
				return nil, err
			}
			if candidate, found := nextUpCandidateForEpisodes(season.Episodes, hist, true); found {
				hits = append(hits, embyNextUpSeriesHit{group: season.Series, candidate: candidate})
			}
		}
	case len(p.SeriesIDs) > 0:
		for _, seriesID := range p.SeriesIDs {
			group, ok, err := e.findSeriesGroup(ctx, seriesID, p.UserID)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			hist, err := e.historyByMediaIDs(ctx, p.UserID, episodeMediaIDs(group.Episodes))
			if err != nil {
				return nil, err
			}
			if candidate, found := nextUpCandidateForEpisodes(group.Episodes, hist, true); found {
				hits = append(hits, embyNextUpSeriesHit{group: group, candidate: candidate})
			}
		}
	default:
		groups, err := e.nextUpVisibleSeriesGroups(ctx, p.UserID)
		if err != nil {
			return nil, err
		}
		allIDs := make([]string, 0, len(groups)*4)
		for _, group := range groups {
			allIDs = append(allIDs, episodeMediaIDs(group.Episodes)...)
		}
		hist, err := e.historyByMediaIDs(ctx, p.UserID, allIDs)
		if err != nil {
			return nil, err
		}
		for _, group := range groups {
			// Home NextUp only advertises series the user actually started;
			// untouched series would flood the row with first episodes.
			if candidate, found := nextUpCandidateForEpisodes(group.Episodes, hist, false); found && candidate.anchored {
				hits = append(hits, embyNextUpSeriesHit{group: group, candidate: candidate})
			}
		}
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if !hits[i].candidate.anchor.Equal(hits[j].candidate.anchor) {
			return hits[i].candidate.anchor.After(hits[j].candidate.anchor)
		}
		return hits[i].group.ID < hits[j].group.ID
	})

	total := len(hits)
	rows := make([]model.Media, 0, len(hits))
	for _, hit := range pageSlice(hits, p.StartIndex, p.Limit) {
		rows = append(rows, hit.candidate.episode)
	}
	items, err := e.payloadsForMedia(ctx, rows, p.UserID, true)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		embyDecorateEpisodeRowTitle(item)
	}
	if err := e.attachResumeSeriesArtwork(ctx, p.UserID, items); err != nil {
		return nil, err
	}
	return map[string]any{"Items": items, "TotalRecordCount": total, "StartIndex": p.StartIndex}, nil
}

// nextUpVisibleSeriesGroups loads every series visible to the user with the
// same grouping the Seasons/Episodes endpoints use, so home NextUp rows agree
// with what clients see on the series page.
func (e *EmbyService) nextUpVisibleSeriesGroups(ctx context.Context, userID string) ([]embySeriesGroup, error) {
	var rows []model.Media
	q := e.repo.DB.WithContext(ctx).Model(&model.Media{}).Where("season_num > 0 OR episode_num > 0")
	q = e.applyUserMediaVisibility(ctx, q, userID)
	if err := q.Order("media.season_num asc, media.episode_num asc, media.created_at asc").
		Limit(embySeriesGroupingLimit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return e.seriesGroupsFromMedia(rows), nil
}

// nextUpCandidateForEpisodes picks the "next up" episode for one series from
// the user's playback history:
//  1. the most recently touched in-progress episode (not fully played with a
//     non-zero position), so clients offer to resume what the user was
//     actually watching last;
//  2. otherwise the first not-fully-played episode after the latest
//     completed one;
//  3. otherwise, when the series has no history at all and the client asked
//     for this series explicitly, the first episode.
//
// episodes must already be ordered by season and episode number, which is how
// the series grouping returns them.
func nextUpCandidateForEpisodes(episodes []model.Media, hist map[string]model.PlaybackHistory, fallbackToFirst bool) (embyNextUpCandidate, bool) {
	if len(episodes) == 0 {
		return embyNextUpCandidate{}, false
	}
	var inProgress, lastPlayed *model.PlaybackHistory
	lastPlayedIndex := -1
	for i := range episodes {
		row, ok := hist[episodes[i].ID]
		if !ok {
			continue
		}
		if embyHistoryRowFullyPlayed(row) {
			if lastPlayed == nil || row.WatchedAt.After(lastPlayed.WatchedAt) {
				row := row
				lastPlayed = &row
				lastPlayedIndex = i
			}
			continue
		}
		if row.PositionMs > 0 && (inProgress == nil || row.WatchedAt.After(inProgress.WatchedAt)) {
			row := row
			inProgress = &row
		}
	}
	if inProgress != nil {
		for i := range episodes {
			if episodes[i].ID == inProgress.MediaID {
				return embyNextUpCandidate{episode: episodes[i], anchor: inProgress.WatchedAt, anchored: true}, true
			}
		}
	}
	if lastPlayed != nil && lastPlayedIndex >= 0 {
		for i := lastPlayedIndex + 1; i < len(episodes); i++ {
			row, watched := hist[episodes[i].ID]
			if watched && embyHistoryRowFullyPlayed(row) {
				continue
			}
			anchor := time.Time{}
			if watched {
				anchor = row.WatchedAt
			}
			return embyNextUpCandidate{episode: episodes[i], anchor: anchor, anchored: true}, true
		}
		return embyNextUpCandidate{}, false
	}
	if fallbackToFirst && lastPlayed == nil && inProgress == nil {
		return embyNextUpCandidate{episode: episodes[0], anchored: false}, true
	}
	return embyNextUpCandidate{}, false
}

// embyHistoryRowFullyPlayed mirrors the payload rule: a row counts as played
// when the writer flagged it completed or the position passed 90% of the
// recorded duration.
func embyHistoryRowFullyPlayed(row model.PlaybackHistory) bool {
	if row.Completed {
		return true
	}
	return row.PositionMs > 0 && row.DurationMs > 0 && row.PositionMs >= row.DurationMs*9/10
}

func episodeMediaIDs(episodes []model.Media) []string {
	ids := make([]string, 0, len(episodes))
	for _, episode := range episodes {
		if id := strings.TrimSpace(episode.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

// historyByMediaIDs loads the user's newest history row per media. Rows are
// queried in batches so large episode sets stay within host-parameter limits.
func (e *EmbyService) historyByMediaIDs(ctx context.Context, userID string, mediaIDs []string) (map[string]model.PlaybackHistory, error) {
	out := make(map[string]model.PlaybackHistory, len(mediaIDs))
	for start := 0; start < len(mediaIDs); start += embyNextUpQueryBatch {
		end := start + embyNextUpQueryBatch
		if end > len(mediaIDs) {
			end = len(mediaIDs)
		}
		var rows []model.PlaybackHistory
		if err := e.repo.DB.WithContext(ctx).
			Where("user_id = ?", userID).
			Where("media_id IN ?", mediaIDs[start:end]).
			Order("watched_at DESC, updated_at DESC, id DESC").
			Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if _, exists := out[row.MediaID]; !exists {
				out[row.MediaID] = row
			}
		}
	}
	return out, nil
}
