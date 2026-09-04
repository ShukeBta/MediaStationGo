package service

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

func SubscriptionTargetLocalAvailability(ctx context.Context, repo *repository.Container, sub *model.Subscription) (LocalAvailability, error) {
	values, err := subscriptionTargetLocalAvailabilities(ctx, repo, []*model.Subscription{sub})
	if err != nil {
		return newSubscriptionSeasonAvailability(sub), err
	}
	return values[0], nil
}

type subscriptionTargetMediaKey struct {
	libraryID     string
	libraryRootID string
}

func subscriptionTargetLocalAvailabilities(ctx context.Context, repo *repository.Container, subs []*model.Subscription) ([]LocalAvailability, error) {
	values := make([]LocalAvailability, len(subs))
	rowsByTarget := make(map[subscriptionTargetMediaKey][]model.Media)
	for i, sub := range subs {
		values[i] = newSubscriptionSeasonAvailability(sub)
		if repo == nil || repo.DB == nil || sub == nil || strings.TrimSpace(sub.LibraryID) == "" || strings.TrimSpace(sub.LibraryRootID) == "" {
			continue
		}
		if len(subscriptionTitleMatchQueries(sub)) == 0 {
			continue
		}
		key := subscriptionTargetMediaKey{
			libraryID:     strings.TrimSpace(sub.LibraryID),
			libraryRootID: strings.TrimSpace(sub.LibraryRootID),
		}
		rows, loaded := rowsByTarget[key]
		if !loaded {
			if err := repo.DB.WithContext(ctx).
				Where("library_id = ? AND library_root_id = ?", key.libraryID, key.libraryRootID).
				Order("season_num ASC, episode_num ASC, created_at DESC").
				Limit(10000).Find(&rows).Error; err != nil {
				return nil, err
			}
			rowsByTarget[key] = rows
		}
		values[i] = subscriptionTargetLocalAvailabilityFromRows(sub, rows)
	}
	return values, nil
}

func subscriptionTargetLocalAvailabilityFromRows(sub *model.Subscription, rows []model.Media) LocalAvailability {
	out := newSubscriptionSeasonAvailability(sub)
	queries := subscriptionTitleMatchQueries(sub)
	if len(queries) == 0 {
		return out
	}
	targetSeason := subscriptionSeasonNumber(sub)
	for _, row := range rows {
		text := strings.TrimSpace(strings.Join([]string{row.Title, row.OriginalName, row.EpisodeTitle, row.Path}, " "))
		if !availabilityTitleMatchesAny(text, queries) {
			continue
		}
		out.LocalMediaCount++
		out.InLibrary = true
		season := row.SeasonNum
		if season <= 0 {
			season = 1
		}
		if season == targetSeason && out.Media == nil {
			representative := row
			out.Media = &representative
			out.MediaID = representative.ID
		}
		if row.EpisodeNum <= 0 {
			if targetSeason == 1 && (row.SeasonNum <= 1) {
				out.HasSeriesPack = true
			}
			continue
		}
		if season != targetSeason {
			continue
		}
		out.ExistingEpisodeKeys[episodeKey(targetSeason, row.EpisodeNum)] = struct{}{}
	}
	return finalizeSubscriptionSeasonAvailability(sub, out)
}

func SubscriptionTargetOpenListPath(ctx context.Context, repo *repository.Container, sub *model.Subscription) (string, error) {
	if repo == nil || repo.DB == nil || sub == nil {
		return "", errors.New("追更正式目录不可用")
	}
	queries := subscriptionTitleMatchQueries(sub)
	if len(queries) == 0 {
		return "", errors.New("追更订阅缺少可识别的作品名称")
	}
	var rows []model.Media
	if err := repo.DB.WithContext(ctx).
		Where("library_id = ? AND library_root_id = ?", sub.LibraryID, sub.LibraryRootID).
		Order("created_at ASC").Limit(10000).Find(&rows).Error; err != nil {
		return "", err
	}
	season := subscriptionSeasonNumber(sub)
	directories := map[string]struct{}{}
	for _, row := range rows {
		itemSeason := row.SeasonNum
		if itemSeason <= 0 {
			itemSeason = 1
		}
		if row.EpisodeNum <= 0 || itemSeason != season {
			continue
		}
		text := strings.TrimSpace(strings.Join([]string{row.Title, row.OriginalName, row.EpisodeTitle, row.Path}, " "))
		if !availabilityTitleMatchesAny(text, queries) {
			continue
		}
		openListPath := pipelineCloudPathToOpenListPath(row.Path)
		if openListPath != "" {
			directories[path.Dir(openListPath)] = struct{}{}
		}
	}
	if len(directories) != 1 {
		if len(directories) == 0 {
			return "", errors.New("未找到现有剧集的唯一正式季目录")
		}
		return "", errors.New("现有剧集分布在多个正式目录，自动追更已拒绝")
	}
	for directory := range directories {
		return directory, nil
	}
	return "", errors.New("追更正式目录不可用")
}

func newSubscriptionSeasonAvailability(sub *model.Subscription) LocalAvailability {
	total := 0
	if sub != nil {
		total = sub.TotalEpisodes
	}
	return LocalAvailability{
		TotalEpisodes:       total,
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
}

func finalizeSubscriptionSeasonAvailability(sub *model.Subscription, out LocalAvailability) LocalAvailability {
	season := subscriptionSeasonNumber(sub)
	out.DownloadedEpisodes = len(out.ExistingEpisodeKeys)
	out.MissingEpisodes = missingEpisodesForSeason(out.ExistingEpisodeKeys, out.TotalEpisodes, season)
	out.MissingEpisodeKeys = map[string]struct{}{}
	for _, episode := range out.MissingEpisodes {
		out.MissingEpisodeKeys[episodeKey(season, episode)] = struct{}{}
	}
	return out
}

func subscriptionAvailabilityNeedsCatchUp(availability LocalAvailability, season int) bool {
	if len(availability.MissingEpisodes) > 0 {
		return true
	}
	if len(availability.ExistingEpisodeKeys) == 0 {
		return false
	}
	frontier := firstUnavailableEpisode(availability.ExistingEpisodeKeys, season)
	return maxAvailabilityEpisode(availability.ExistingEpisodeKeys) > frontier
}

func mergeLocalAvailabilityForSeason(season int, values ...LocalAvailability) LocalAvailability {
	if season <= 0 {
		season = 1
	}
	out := LocalAvailability{
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
	for _, value := range values {
		if value.TotalEpisodes > out.TotalEpisodes {
			out.TotalEpisodes = value.TotalEpisodes
		}
		out.LocalMediaCount += value.LocalMediaCount
		out.InLibrary = out.InLibrary || value.InLibrary
		out.HasSeriesPack = out.HasSeriesPack || value.HasSeriesPack
		if out.MediaID == "" {
			out.MediaID = value.MediaID
		}
		if out.Media == nil && value.Media != nil {
			out.Media = value.Media
		}
		for key := range value.ExistingEpisodeKeys {
			out.ExistingEpisodeKeys[key] = struct{}{}
		}
	}
	out.DownloadedEpisodes = len(out.ExistingEpisodeKeys)
	out.MissingEpisodes = missingEpisodesForSeason(out.ExistingEpisodeKeys, out.TotalEpisodes, season)
	for _, episode := range out.MissingEpisodes {
		out.MissingEpisodeKeys[episodeKey(season, episode)] = struct{}{}
	}
	return out
}

func missingEpisodesForSeason(existing map[string]struct{}, total, season int) []int {
	if total <= 0 {
		return nil
	}
	if season <= 0 {
		season = 1
	}
	missing := make([]int, 0, total)
	for episode := 1; episode <= total; episode++ {
		if _, ok := existing[episodeKey(season, episode)]; ok {
			continue
		}
		missing = append(missing, episode)
	}
	return missing
}
