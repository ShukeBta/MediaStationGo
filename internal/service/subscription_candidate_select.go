package service

import (
	"sort"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func selectPreparedSubscriptionCandidates(candidates []siteSearchCandidate, sub *model.Subscription, local LocalAvailability) []siteSearchCandidate {
	return selectPreparedSubscriptionCandidatesWithStats(candidates, sub, local, nil)
}

func selectPreparedSubscriptionCandidatesWithStats(candidates []siteSearchCandidate, sub *model.Subscription, local LocalAvailability, stats *siteSearchSelectionStats) []siteSearchCandidate {
	if len(candidates) > 1 {
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].Score != candidates[j].Score {
				return candidates[i].Score > candidates[j].Score
			}
			if candidates[i].Item.Seeders != candidates[j].Item.Seeders {
				return candidates[i].Item.Seeders > candidates[j].Item.Seeders
			}
			return candidates[i].Item.Size > candidates[j].Item.Size
		})
	}
	if len(candidates) == 0 {
		return recordPreparedSelection(nil, stats)
	}

	mediaType := normalizeMediaType(sub.MediaType, sub.Name+" "+sub.Filter, "")
	if !isSubscriptionSeriesType(mediaType) {
		// 非洗版订阅成功下载一次即满足，媒体库/下载中已存在则不再重复下载。
		if !subscriptionAllowsWash(sub) && local.LocalMediaCount > 0 {
			if stats != nil {
				stats.LocalAlreadySatisfied = true
			}
			return recordPreparedSelection(nil, stats)
		}
		return recordPreparedSelection(candidates[:1], stats)
	}

	if localSeriesPackSatisfiesSubscription(local) {
		if stats != nil {
			stats.LocalSeriesPackPresent = true
		}
		return recordPreparedSelection(nil, stats)
	}
	if local.LocalMediaCount > 0 {
		trustedTotal := trustedAvailabilityTotal(local)
		if trustedTotal > 0 && len(local.MissingEpisodes) == 0 {
			if stats != nil {
				stats.SeriesComplete = true
			}
			return recordPreparedSelection(nil, stats)
		}
		missingSet := missingEpisodeSet(local)
		onlyMissing := make([]siteSearchCandidate, 0, len(candidates))
		var packFallback *siteSearchCandidate
		for i := range candidates {
			candidate := candidates[i]
			if candidate.Episode <= 0 {
				// 整季/全集包(无单集号)。剧集完结后站点常只挂全集包,
				// 这里记下来作兜底:当单集候选不足以补齐缺失集时启用,
				// 否则"补全缺失集"在站点只有全集包时永远匹配为空。
				if stats != nil {
					stats.NoEpisodeSkipped++
				}
				if candidate.Pack && packFallback == nil {
					packFallback = &candidates[i]
					if stats != nil {
						stats.PackFallbackAvailable = true
					}
				}
				continue
			}
			season := candidate.Season
			if season <= 0 {
				season = 1
			}
			if candidateEpisodesAllExist(local.ExistingEpisodeKeys, season, candidate) {
				if stats != nil {
					stats.ExistingEpisodeSkipped++
				}
				continue
			}
			if trustedTotal > 0 && !candidateCoversMissingEpisode(candidate, missingSet) {
				if stats != nil {
					stats.NotMissingEpisodeSkipped++
				}
				continue
			}
			onlyMissing = append(onlyMissing, candidate)
		}
		selected := sortedEpisodeCandidates(onlyMissing)
		if len(selected) == 0 && packFallback != nil {
			// 没有可用的单集候选,但站点有整季/全集包 → 用包兜底补缺集。
			// 代价是会重下已有集,但用户主动触发补全时这是可接受的。
			if stats != nil {
				stats.PackFallbackUsed = true
			}
			return recordPreparedSelection([]siteSearchCandidate{*packFallback}, stats)
		}
		return recordPreparedSelection(selected, stats)
	}

	for _, candidate := range candidates {
		if candidate.Pack {
			return recordPreparedSelection([]siteSearchCandidate{candidate}, stats)
		}
	}

	selected := sortedEpisodeCandidates(candidates)
	if len(selected) == 0 {
		return recordPreparedSelection(candidates[:1], stats)
	}
	return recordPreparedSelection(selected, stats)
}

func candidateEpisodesAllExist(existing map[string]struct{}, season int, candidate siteSearchCandidate) bool {
	episodes := candidateEpisodeNumbers(candidate)
	if len(episodes) == 0 {
		return false
	}
	for _, episode := range episodes {
		if _, exists := existing[episodeKey(season, episode)]; !exists {
			return false
		}
	}
	return true
}

func candidateCoversMissingEpisode(candidate siteSearchCandidate, missingSet map[int]struct{}) bool {
	episodes := candidateEpisodeNumbers(candidate)
	if len(episodes) == 0 {
		return false
	}
	for _, episode := range episodes {
		if _, missing := missingSet[episode]; missing {
			return true
		}
	}
	return false
}

func candidateEpisodeNumbers(candidate siteSearchCandidate) []int {
	if len(candidate.Episodes) > 0 {
		return candidate.Episodes
	}
	if candidate.Episode > 0 {
		return []int{candidate.Episode}
	}
	return nil
}

func recordPreparedSelection(candidates []siteSearchCandidate, stats *siteSearchSelectionStats) []siteSearchCandidate {
	if stats != nil {
		stats.Selected = len(candidates)
	}
	return candidates
}

func localSeriesPackSatisfiesSubscription(local LocalAvailability) bool {
	if !local.HasSeriesPack {
		return false
	}
	total := trustedAvailabilityTotal(local)
	if total <= 0 {
		return len(local.ExistingEpisodeKeys) == 0
	}
	if len(local.MissingEpisodes) > 0 {
		return false
	}
	return len(local.ExistingEpisodeKeys) >= total
}

func trustedAvailabilityTotal(local LocalAvailability) int {
	total := local.TotalEpisodes
	if total <= 0 {
		return 0
	}
	if maxEpisode := maxAvailabilityEpisode(local.ExistingEpisodeKeys); maxEpisode > total {
		return 0
	}
	return total
}
