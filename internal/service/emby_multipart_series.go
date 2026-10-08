package service

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

const manualSeriesEpisodeSQL = "COALESCE(part_group_key, '') <> '' AND part_group_key NOT LIKE 'auto-part:%' AND episode_num > 0 AND season_num >= 0"
const embyLogicalEpisodeSQL = "CASE WHEN " + manualSeriesEpisodeSQL + " THEN 'episode:' || CAST(season_num AS TEXT) || ':' || CAST(episode_num AS TEXT) || ':' || CAST(episode_end_num AS TEXT) || ':' || CAST(episode_part_num AS TEXT) WHEN COALESCE(part_group_key, '') = '' THEN COALESCE(NULLIF(emby_version_key, ''), 'row:' || id) ELSE 'row:' || id END"

func multipartSeriesID(libraryID, partGroupKey string) string {
	return stableEmbyID(embyVirtualSeriesPrefix, libraryID, "multipart", partGroupKey)
}

func (e *EmbyService) multipartSeriesGroupsFromMedia(rows []model.Media) []embySeriesGroup {
	byKey := make(map[string][]model.Media)
	order := make([]string, 0)
	for _, row := range rows {
		key := strings.TrimSpace(row.PartGroupKey)
		if key == "" {
			continue
		}
		groupKey := strings.ToLower(strings.TrimSpace(row.LibraryID)) + "\x00" + strings.ToLower(key)
		if _, exists := byKey[groupKey]; !exists {
			order = append(order, groupKey)
		}
		byKey[groupKey] = append(byKey[groupKey], row)
	}

	groups := make([]embySeriesGroup, 0, len(order))
	for _, key := range order {
		parts := byKey[key]
		sort.SliceStable(parts, func(i, j int) bool { return betterMediaPart(parts[i], parts[j]) })
		if len(parts) == 0 {
			continue
		}
		anchor := parts[0]
		posterURL, backdropURL := multipartArtworkForMedia(anchor)
		group := embySeriesGroup{
			ID:          multipartSeriesID(anchor.LibraryID, anchor.PartGroupKey),
			LibraryID:   anchor.LibraryID,
			Name:        firstNonEmpty(anchor.PartGroupTitle, anchor.Title),
			PosterURL:   posterURL,
			BackdropURL: backdropURL,
			Overview:    anchor.Overview,
			Rating:      anchor.Rating,
			Year:        anchor.Year,
			ReleaseDate: anchor.ReleaseDate,
			TMDbID:      anchor.TMDbID,
			BangumiID:   anchor.BangumiID,
			Genres:      e.embyGenresForMedia(&anchor, ""),
			CreatedAt:   anchor.CreatedAt,
			Episodes:    make([]model.Media, 0, len(parts)),
		}
		logicalEpisodes := make(map[string]int)
		for index := range parts {
			part := multipartEpisodeView(parts[index], group.ID, index+1)
			partPosterURL, partBackdropURL := multipartArtworkForMedia(part)
			if part.CreatedAt.After(group.CreatedAt) {
				group.CreatedAt = part.CreatedAt
			}
			if group.PosterURL == "" && partPosterURL != "" {
				group.PosterURL = partPosterURL
			}
			if group.BackdropURL == "" && partBackdropURL != "" {
				group.BackdropURL = partBackdropURL
			}
			if group.Overview == "" && part.Overview != "" {
				group.Overview = part.Overview
			}
			group.Genres = uniqueFoldedStrings(append(group.Genres, e.embyGenresForMedia(&part, "")...))
			if manualSeriesEpisode(part) {
				key := manualSeriesEpisodeVersionKey(part)
				if position, exists := logicalEpisodes[key]; exists {
					if preferMediaVersion(part, group.Episodes[position]) {
						group.Episodes[position] = part
					}
					continue
				}
				logicalEpisodes[key] = len(group.Episodes)
			}
			group.Episodes = append(group.Episodes, part)
		}
		groups = append(groups, group)
	}
	return groups
}

func multipartArtworkForMedia(row model.Media) (string, string) {
	posterURL := firstNonEmpty(row.PosterURL, row.GeneratedPosterURL)
	backdropURL := firstNonEmpty(row.BackdropURL, row.GeneratedBackdropURL)
	return posterURL, backdropURL
}

func multipartEpisodeView(row model.Media, seriesID string, episodeNumber int) model.Media {
	if episodeNumber <= 0 {
		episodeNumber = 1
	}
	originalTitle := strings.TrimSpace(row.Title)
	row.SeriesID = seriesID
	if !manualSeriesEpisode(row) {
		row.SyntheticPartEpisode = true
		row.SeasonNum = 1
		row.EpisodeNum = episodeNumber
	}
	row.EpisodeTitle = firstNonEmpty(row.EpisodeTitle, originalTitle, row.PartGroupTitle)
	row.PartCount = 0
	return row
}

func manualSeriesEpisode(row model.Media) bool {
	key := strings.TrimSpace(row.PartGroupKey)
	return key != "" && !strings.HasPrefix(key, autoMediaPartPrefix) && row.EpisodeNum > 0 && row.SeasonNum >= 0 && !row.SyntheticPartEpisode
}

func manualSeriesEpisodeVersionKey(row model.Media) string {
	return fmt.Sprintf("manual:%s:%s|s:%d|e:%d|end:%d|part:%d", row.LibraryID, row.PartGroupKey, row.SeasonNum, row.EpisodeNum, row.EpisodeEndNum, row.EpisodePartNum)
}

func (e *EmbyService) multipartEpisodeForMedia(ctx context.Context, row model.Media) model.Media {
	if e == nil || e.repo == nil || e.repo.DB == nil || strings.TrimSpace(row.PartGroupKey) == "" {
		return row
	}
	var parts []model.Media
	if err := e.repo.DB.WithContext(ctx).Model(&model.Media{}).
		Where("library_id = ? AND part_group_key = ?", row.LibraryID, row.PartGroupKey).
		Order("CASE WHEN part_index > 0 THEN part_index ELSE 2147483647 END ASC").
		Order("media.created_at ASC").Order("media.id ASC").Find(&parts).Error; err != nil {
		return row
	}
	seriesID := multipartSeriesID(row.LibraryID, row.PartGroupKey)
	for index := range parts {
		if parts[index].ID == row.ID {
			return multipartEpisodeView(row, seriesID, index+1)
		}
	}
	return multipartEpisodeView(row, seriesID, 1)
}
