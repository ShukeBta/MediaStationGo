package service

import (
	"context"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func addAvailabilityTitle(title, query string, out *LocalAvailability) {
	if out == nil || strings.TrimSpace(title) == "" || strings.TrimSpace(query) == "" {
		return
	}
	if !availabilityTitleMatchesAny(title, []string{query}) {
		return
	}
	out.LocalMediaCount++
	if refs := episodeRefsFromTitle(title); len(refs) > 0 {
		if out.ExistingEpisodeKeys == nil {
			out.ExistingEpisodeKeys = map[string]struct{}{}
		}
		for _, ref := range refs {
			out.ExistingEpisodeKeys[episodeKey(ref.Season, ref.Episode)] = struct{}{}
		}
		return
	}
	if isSeriesPackTitle(title) {
		out.HasSeriesPack = true
	}
}

func addAvailabilityTitleAny(title string, queries []string, out *LocalAvailability) {
	if !availabilityTitleMatchesAny(title, queries) {
		return
	}
	addTrustedAvailabilityTitle(title, 0, 0, false, out)
}

func availabilityTitleMatchesAny(title string, queries []string) bool {
	titleKey := normalizeAvailabilityComparable(title)
	if titleKey == "" {
		return false
	}
	for _, query := range queries {
		queryKey := normalizeAvailabilityComparable(query)
		if queryKey == "" {
			continue
		}
		if strings.Contains(titleKey, queryKey) {
			return true
		}
	}
	return false
}

func addSiteSearchCandidateAvailability(candidate siteSearchCandidate, out *LocalAvailability) {
	addTrustedAvailabilityTitle(subscriptionSearchResultText(candidate.Item), candidate.Season, candidate.Episode, candidate.Pack, out)
}

func (s *SubscriptionService) subscriptionCandidateConfirmedAvailable(ctx context.Context, sub *model.Subscription, candidate siteSearchCandidate) bool {
	availability := mergeLocalAvailability(
		SubscriptionLocalAvailability(ctx, s.repo, sub),
		s.pendingDownloadAvailability(ctx, sub),
	)
	return candidateAvailableInAvailability(sub, candidate, availability)
}

func candidateAvailableInAvailability(sub *model.Subscription, candidate siteSearchCandidate, availability LocalAvailability) bool {
	mediaType := normalizeMediaType(subscriptionMediaType(sub), subscriptionName(sub)+" "+subscriptionFilter(sub), "")
	if !isSubscriptionSeriesType(mediaType) {
		return availability.LocalMediaCount > 0 || availability.InLibrary
	}
	episodes := candidateEpisodeNumbers(candidate)
	if len(episodes) == 0 {
		return availability.HasSeriesPack
	}
	season := candidate.Season
	if season <= 0 {
		season = 1
	}
	for _, episode := range episodes {
		if _, ok := availability.ExistingEpisodeKeys[episodeKey(season, episode)]; !ok {
			return false
		}
	}
	return true
}

func addTrustedAvailabilityTitle(title string, season, episode int, pack bool, out *LocalAvailability) {
	if out == nil {
		return
	}
	if strings.TrimSpace(title) == "" && episode <= 0 && !pack {
		return
	}
	out.LocalMediaCount++
	refs := episodeRefsFromTitle(title)
	if len(refs) == 0 && episode > 0 {
		if season <= 0 {
			season = 1
		}
		refs = []episodeRef{{Season: season, Episode: episode}}
	}
	if len(refs) > 0 {
		if out.ExistingEpisodeKeys == nil {
			out.ExistingEpisodeKeys = map[string]struct{}{}
		}
		for _, ref := range refs {
			out.ExistingEpisodeKeys[episodeKey(ref.Season, ref.Episode)] = struct{}{}
		}
		return
	}
	if episode <= 0 {
		season, episode = ParseEpisode(title)
	}
	if episode > 0 {
		if out.ExistingEpisodeKeys == nil {
			out.ExistingEpisodeKeys = map[string]struct{}{}
		}
		out.ExistingEpisodeKeys[episodeKey(season, episode)] = struct{}{}
		return
	}
	if pack || isSeriesPackTitle(title) {
		out.HasSeriesPack = true
	}
}

func mergeLocalAvailability(values ...LocalAvailability) LocalAvailability {
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
		for key := range value.ExistingEpisodeKeys {
			out.ExistingEpisodeKeys[key] = struct{}{}
		}
	}
	out.DownloadedEpisodes = len(out.ExistingEpisodeKeys)
	if out.TotalEpisodes > 0 {
		out.MissingEpisodes = missingEpisodes(out.ExistingEpisodeKeys, out.TotalEpisodes)
		for _, episode := range out.MissingEpisodes {
			out.MissingEpisodeKeys[episodeKey(1, episode)] = struct{}{}
		}
	}
	if out.DownloadedEpisodes == 0 && out.LocalMediaCount > 0 {
		out.DownloadedEpisodes = out.LocalMediaCount
		if out.TotalEpisodes == 0 {
			out.TotalEpisodes = 1
		}
	}
	return out
}

// subscriptionItemAlreadyAvailable 判断某个订阅条目（按其标题解析出的季/集）是否已在媒体库存在。
// 电影/无集号条目：媒体库已有该片即视为已存在；剧集条目：对应季集已入库即视为已存在。
func subscriptionItemAlreadyAvailable(sub *model.Subscription, avail LocalAvailability, title string) bool {
	if avail.LocalMediaCount == 0 && !avail.HasSeriesPack {
		return false
	}
	if !isSubscriptionSeriesType(subscriptionMediaType(sub)) {
		return true
	}
	if avail.HasSeriesPack {
		return true
	}
	wantSeason, wantEpisode := ParseEpisode(title)
	if wantEpisode <= 0 {
		// 整季合集 / 无法解析集号：库里已有内容时保守跳过，避免重复整季下载。
		return true
	}
	if wantSeason <= 0 {
		wantSeason = 1
	}
	_, exists := avail.ExistingEpisodeKeys[episodeKey(wantSeason, wantEpisode)]
	return exists
}
