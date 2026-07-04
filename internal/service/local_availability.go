package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"github.com/ShukeBta/MediaStationGo/internal/repository"
)

var availabilityNoiseRE = regexp.MustCompile(`(?i)(自动订阅|订阅|全集|合集|complete|batch|s\d{1,2}e\d{1,3}|season\s*\d+|s\d{1,2}|第\s*\d+\s*季|第\s*\d+\s*[集话話期]|\(\d{4}\)|\b\d{4}\b|2160p|1080p|720p|4k|uhd|bluray|blu-ray|web-?dl|hdtv|remux|x26[45]|h\.?26[45]|hevc|avc|hdr10?\+?|dovi|dv|atmos|aac|ddp?5\.1|truehd|flac)`)

type LocalAvailability struct {
	DownloadedEpisodes  int
	TotalEpisodes       int
	LocalMediaCount     int
	MissingEpisodes     []int
	InLibrary           bool
	HasSeriesPack       bool
	ExistingEpisodeKeys map[string]struct{}
	MissingEpisodeKeys  map[string]struct{}
}

func EnrichExternalMediaAvailability(ctx context.Context, repo *repository.Container, items []ExternalMediaResult) {
	for i := range items {
		availability := LookupLocalAvailability(ctx, repo, items[i].Title, items[i].SubscribeKeyword, items[i].MediaType, items[i].TotalEpisodes)
		items[i].DownloadedEpisodes = availability.DownloadedEpisodes
		items[i].LocalMediaCount = availability.LocalMediaCount
		items[i].MissingEpisodes = availability.MissingEpisodes
		items[i].InLibrary = availability.InLibrary
		if items[i].TotalEpisodes == 0 {
			items[i].TotalEpisodes = availability.TotalEpisodes
		}
	}
}

func EnrichSubscriptionProgress(ctx context.Context, repo *repository.Container, items []model.Subscription) {
	for i := range items {
		availability := SubscriptionLocalAvailability(ctx, repo, &items[i])
		items[i].DownloadedEpisodes = availability.DownloadedEpisodes
		items[i].LocalMediaCount = availability.LocalMediaCount
		items[i].MissingEpisodes = availability.MissingEpisodes
		items[i].InLibrary = availability.InLibrary
		if items[i].TotalEpisodes == 0 {
			items[i].TotalEpisodes = availability.TotalEpisodes
		}
	}
}

func SubscriptionLocalAvailability(ctx context.Context, repo *repository.Container, sub *model.Subscription) LocalAvailability {
	if sub == nil {
		return LocalAvailability{}
	}
	expected := sub.TotalEpisodes
	return LookupLocalAvailability(ctx, repo, sub.Name, sub.Filter, sub.MediaType, expected)
}

func LookupLocalAvailability(ctx context.Context, repo *repository.Container, title, keyword, mediaType string, expectedTotal int) LocalAvailability {
	out := LocalAvailability{
		TotalEpisodes:       expectedTotal,
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
	if repo == nil || repo.DB == nil {
		return out
	}
	query := availabilityQuery(title, keyword)
	if query == "" {
		return out
	}
	like := "%" + query + "%"
	var rows []model.Media
	if err := repo.DB.WithContext(ctx).
		Where("title LIKE ? OR original_name LIKE ? OR path LIKE ?", like, like, like).
		Order("season_num asc, episode_num asc, created_at desc").
		Limit(2000).
		Find(&rows).Error; err != nil {
		return out
	}
	out.LocalMediaCount = len(rows)
	out.InLibrary = len(rows) > 0
	if len(rows) == 0 {
		return out
	}

	seriesLike := isSubscriptionSeriesType(mediaType)
	for _, row := range rows {
		if row.EpisodeNum <= 0 {
			if seriesLike {
				out.HasSeriesPack = true
			}
			continue
		}
		season := row.SeasonNum
		if season <= 0 {
			season = 1
		}
		key := episodeKey(season, row.EpisodeNum)
		out.ExistingEpisodeKeys[key] = struct{}{}
	}
	if seriesLike || len(out.ExistingEpisodeKeys) > 0 {
		out.DownloadedEpisodes = len(out.ExistingEpisodeKeys)
		out.MissingEpisodes = missingEpisodes(out.ExistingEpisodeKeys, out.TotalEpisodes)
		for _, episode := range out.MissingEpisodes {
			out.MissingEpisodeKeys[episodeKey(1, episode)] = struct{}{}
		}
		return out
	}
	out.DownloadedEpisodes = 1
	if out.TotalEpisodes == 0 {
		out.TotalEpisodes = 1
	}
	return out
}

func missingEpisodes(existing map[string]struct{}, total int) []int {
	if total <= 0 {
		return nil
	}
	missing := make([]int, 0)
	for episode := 1; episode <= total; episode++ {
		if _, ok := existing[episodeKey(1, episode)]; ok {
			continue
		}
		missing = append(missing, episode)
	}
	return missing
}

func availabilityQuery(title, keyword string) string {
	for _, candidate := range []string{keyword, title} {
		cleaned := cleanAvailabilityTitle(candidate)
		if cleaned != "" {
			return cleaned
		}
	}
	return ""
}

func cleanAvailabilityTitle(value string) string {
	value = availabilityNoiseRE.ReplaceAllString(value, " ")
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	value = strings.TrimSuffix(value, "-")
	value = strings.TrimSpace(value)
	return value
}

func episodeKey(season, episode int) string {
	if season <= 0 {
		season = 1
	}
	return fmt.Sprintf("%02dE%03d", season, episode)
}

func missingEpisodeSet(availability LocalAvailability) map[int]struct{} {
	out := make(map[int]struct{}, len(availability.MissingEpisodes))
	for _, episode := range availability.MissingEpisodes {
		out[episode] = struct{}{}
	}
	return out
}

func sortedEpisodeCandidates(candidates []siteSearchCandidate) []siteSearchCandidate {
	selected := make([]siteSearchCandidate, 0, len(candidates))
	covered := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		keys := candidateEpisodeKeys(candidate)
		if len(keys) == 0 {
			continue
		}
		if episodeKeysOverlap(covered, keys) {
			continue
		}
		selected = append(selected, candidate)
		for _, key := range keys {
			covered[key] = struct{}{}
		}
	}
	sort.SliceStable(selected, func(i, j int) bool {
		return candidateFirstEpisodeKey(selected[i]) < candidateFirstEpisodeKey(selected[j])
	})
	return selected
}

func candidateEpisodeKeys(candidate siteSearchCandidate) []string {
	episodes := candidateEpisodeNumbers(candidate)
	if len(episodes) == 0 {
		return nil
	}
	season := candidate.Season
	if season <= 0 {
		season = 1
	}
	keys := make([]string, 0, len(episodes))
	seen := make(map[string]struct{}, len(episodes))
	for _, episode := range episodes {
		if episode <= 0 {
			continue
		}
		key := episodeKey(season, episode)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func candidateFirstEpisodeKey(candidate siteSearchCandidate) string {
	keys := candidateEpisodeKeys(candidate)
	if len(keys) == 0 {
		return ""
	}
	return keys[0]
}

func episodeKeysOverlap(covered map[string]struct{}, keys []string) bool {
	for _, key := range keys {
		if _, ok := covered[key]; ok {
			return true
		}
	}
	return false
}
