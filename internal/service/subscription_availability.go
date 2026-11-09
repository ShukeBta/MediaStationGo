// Package service — subscription local and pending-download availability helpers.
package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ShukeBta/MediaStationGo/internal/model"
)

func (s *SubscriptionService) pendingDownloadAvailability(ctx context.Context, sub *model.Subscription) LocalAvailability {
	out := LocalAvailability{
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
	if sub != nil {
		out.TotalEpisodes = sub.TotalEpisodes
	}
	queries := subscriptionAvailabilityQueries(sub)
	if len(queries) == 0 {
		return s.finalizePendingAvailability(sub, out)
	}
	root := s.subscriptionBaseSavePath(ctx, sub)
	rejectedReleases := map[string]struct{}{}
	if sub != nil {
		rejectedReleases = s.rejectedReleaseIdentitySet(ctx, sub.ID)
	}
	if root != "" {
		var match func(string) bool
		if sub != nil && sub.TMDbID > 0 {
			match = func(path string) bool { return subscriptionCandidatePathMatchesIdentity(sub, path) }
		}
		_ = scanDownloadPathAnyMatching(ctx, root, queries, match, func(path string, season, episode int) bool {
			releaseTitle := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if _, rejected := rejectedReleases[rejectedReleaseIdentity(releaseTitle, "")]; rejected {
				// A rejected file can remain on disk if a filesystem removal failed;
				// it must not make this subscription believe the episode is repaired.
				return true
			}
			out.LocalMediaCount++
			if refs := episodeRefsFromTitle(path); len(refs) > 0 {
				for _, ref := range refs {
					out.ExistingEpisodeKeys[episodeKey(ref.Season, ref.Episode)] = struct{}{}
				}
			} else if episode > 0 {
				out.ExistingEpisodeKeys[episodeKey(season, episode)] = struct{}{}
			}
			return true
		})
	}
	s.addDownloadTaskAvailability(ctx, sub, queries, &out)
	s.addLiveTorrentAvailability(ctx, sub, queries, &out)
	return s.finalizePendingAvailability(sub, out)
}

func (s *SubscriptionService) EnrichProgress(ctx context.Context, items []model.Subscription) error {
	for i := range items {
		if subscriptionUsesResourceImport(&items[i]) {
			local, err := SubscriptionTargetLocalAvailability(ctx, s.repo, &items[i])
			if err != nil {
				return err
			}
			pending, err := s.pendingResourceImportAvailability(ctx, &items[i])
			if err != nil {
				return err
			}
			availability := mergeLocalAvailabilityForSeason(
				subscriptionSeasonNumber(&items[i]),
				local,
				pending,
			)
			applySubscriptionAvailability(&items[i], availability)
			items[i].CatchUpActive = items[i].CatchUpActive && subscriptionAvailabilityNeedsCatchUp(availability, subscriptionSeasonNumber(&items[i]))
			continue
		}
		availability := mergeLocalAvailability(
			SubscriptionLocalAvailability(ctx, s.repo, &items[i]),
			s.pendingDownloadAvailability(ctx, &items[i]),
		)
		availability = s.finalizePendingAvailability(&items[i], availability)
		applySubscriptionAvailability(&items[i], availability)
	}
	return nil
}

func (s *SubscriptionService) EnrichManagementProgress(ctx context.Context, items []model.Subscription) error {
	rows := s.downloadTaskRowsForAvailability(ctx)
	targetSubs := make([]*model.Subscription, 0, len(items))
	for i := range items {
		if subscriptionUsesResourceImport(&items[i]) {
			targetSubs = append(targetSubs, &items[i])
		}
	}
	targetAvailability, err := subscriptionTargetLocalAvailabilities(ctx, s.repo, targetSubs)
	if err != nil {
		return err
	}
	targetIndex := 0
	for i := range items {
		if subscriptionUsesResourceImport(&items[i]) {
			local := targetAvailability[targetIndex]
			targetIndex++
			pending, err := s.pendingResourceImportAvailability(ctx, &items[i])
			if err != nil {
				return err
			}
			availability := mergeLocalAvailabilityForSeason(
				subscriptionSeasonNumber(&items[i]),
				local,
				pending,
			)
			applySubscriptionAvailability(&items[i], availability)
			items[i].CatchUpActive = items[i].CatchUpActive && subscriptionAvailabilityNeedsCatchUp(availability, subscriptionSeasonNumber(&items[i]))
			continue
		}
		availability := mergeLocalAvailability(
			SubscriptionLocalAvailability(ctx, s.repo, &items[i]),
			s.pendingDownloadTaskAvailability(ctx, &items[i], rows, false),
		)
		availability = s.finalizePendingAvailability(&items[i], availability)
		applySubscriptionAvailability(&items[i], availability)
	}
	return nil
}

func applySubscriptionAvailability(sub *model.Subscription, availability LocalAvailability) {
	if sub == nil {
		return
	}
	sub.DownloadedEpisodes = availability.DownloadedEpisodes
	sub.LocalMediaCount = availability.LocalMediaCount
	sub.MissingEpisodes = availability.MissingEpisodes
	sub.InLibrary = availability.InLibrary
	sub.MediaID = availability.MediaID
	sub.Media = availability.Media
	if availability.Media != nil {
		sub.SeriesKey = mediaSeriesKey(*availability.Media)
	} else {
		sub.SeriesKey = ""
	}
	if sub.TotalEpisodes == 0 {
		sub.TotalEpisodes = availability.TotalEpisodes
	}
}

func (s *SubscriptionService) downloadTaskRowsForAvailability(ctx context.Context) []model.DownloadTask {
	if s == nil || s.repo == nil || s.repo.Download == nil {
		return nil
	}
	rows, err := s.repo.Download.List(ctx)
	if err != nil {
		return nil
	}
	return rows
}

func (s *SubscriptionService) addDownloadTaskAvailability(ctx context.Context, sub *model.Subscription, queries []string, out *LocalAvailability) {
	rows := s.downloadTaskRowsForAvailability(ctx)
	s.addDownloadTaskRowsAvailability(ctx, sub, queries, rows, true, out)
}

func (s *SubscriptionService) pendingDownloadTaskAvailability(ctx context.Context, sub *model.Subscription, rows []model.DownloadTask, verifyLive bool) LocalAvailability {
	out := LocalAvailability{
		ExistingEpisodeKeys: map[string]struct{}{},
		MissingEpisodeKeys:  map[string]struct{}{},
	}
	if sub != nil {
		out.TotalEpisodes = sub.TotalEpisodes
	}
	queries := subscriptionAvailabilityQueries(sub)
	if len(queries) == 0 {
		return s.finalizePendingAvailability(sub, out)
	}
	s.addDownloadTaskRowsAvailability(ctx, sub, queries, rows, verifyLive, &out)
	return s.finalizePendingAvailability(sub, out)
}

func (s *SubscriptionService) addDownloadTaskRowsAvailability(ctx context.Context, sub *model.Subscription, queries []string, rows []model.DownloadTask, verifyLive bool, out *LocalAvailability) {
	if out == nil {
		return
	}
	baseSavePath := s.subscriptionBaseSavePath(ctx, sub)
	rejectedReleases := map[string]struct{}{}
	if sub != nil {
		rejectedReleases = s.rejectedReleaseIdentitySet(ctx, sub.ID)
	}
	for _, row := range rows {
		if !downloadTaskBlocksReadd(row.Status) {
			continue
		}
		if verifyLive && !s.downloadTaskCountsAsPending(ctx, row) {
			continue
		}
		if _, rejected := rejectedReleases[rejectedReleaseIdentity(row.Title, row.IdentityKey)]; rejected {
			continue
		}
		linkedToSubscription := sub != nil && strings.TrimSpace(row.SubscriptionID) != "" && row.SubscriptionID == sub.ID
		if !linkedToSubscription && baseSavePath != "" && row.SavePath != "" && !sameOrChildPath(row.SavePath, baseSavePath) && !sameOrChildPath(baseSavePath, row.SavePath) {
			continue
		}
		if linkedToSubscription {
			season, episode := 0, 0
			if subscriptionUsesSiteSearch(sub) && sub.SeasonNumber > 0 {
				season = sub.SeasonNumber
				if refs := subscriptionCandidateEpisodeRefs(sub, row.Title); len(refs) > 0 {
					episode = refs[0].Episode
				}
			}
			addTrustedAvailabilityTitle(row.Title, season, episode, false, out)
			continue
		}
		addAvailabilityTitleForSubscription(row.Title, sub, queries, out)
	}
}

func (s *SubscriptionService) downloadTaskCountsAsPending(ctx context.Context, row model.DownloadTask) bool {
	if s == nil || s.downloads == nil {
		return true
	}
	// A task row is written only after the downloader accepts the request. The
	// normal path records either the selected client or the downloader's native
	// task id. A bare queued row can be an interrupted pre-submit record, so it
	// still needs a live-torrent check before it occupies an episode. Once the
	// downloader has advanced the row past queued, its persisted state is the
	// authoritative availability signal even if the torrent has since left the
	// live list.
	switch strings.ToLower(strings.TrimSpace(row.Status)) {
	case "queued", "pending", "created", "waiting":
		if strings.TrimSpace(row.DownloadClientID) != "" || strings.TrimSpace(row.ExternalID) != "" {
			return true
		}
		// Legacy synthetic rows (and old rows without a URL) have no live
		// identity to verify; retain their historical status-based behavior.
		if strings.TrimSpace(row.URL) == "" {
			return true
		}
	default:
		return true
	}
	return s.downloads.subscriptionDownloadTaskStillLive(ctx, row)
}

func (s *SubscriptionService) addLiveTorrentAvailability(ctx context.Context, sub *model.Subscription, queries []string, out *LocalAvailability) {
	if s == nil || s.downloads == nil || out == nil {
		return
	}
	live, err := s.downloads.listLiveTorrents(ctx, "")
	if err != nil && len(live) == 0 {
		return
	}
	rejectedReleases := map[string]struct{}{}
	if sub != nil {
		rejectedReleases = s.rejectedReleaseIdentitySet(ctx, sub.ID)
	}
	for _, torrent := range live {
		if _, rejected := rejectedReleases[rejectedReleaseIdentity(torrent.Name, "")]; rejected {
			continue
		}
		addAvailabilityTitleForSubscription(torrent.Name, sub, queries, out)
	}
}

func (s *SubscriptionService) finalizePendingAvailability(sub *model.Subscription, out LocalAvailability) LocalAvailability {
	if subscriptionUsesSiteSearch(sub) && sub.SeasonNumber > 0 && isSubscriptionSeriesType(sub.MediaType) {
		return finalizePTSeasonAvailability(sub, out)
	}
	mediaType := ""
	if sub != nil {
		mediaType = sub.MediaType
	}
	if isSubscriptionSeriesType(mediaType) || len(out.ExistingEpisodeKeys) > 0 {
		out.DownloadedEpisodes = len(out.ExistingEpisodeKeys)
		out.MissingEpisodes = missingEpisodes(out.ExistingEpisodeKeys, out.TotalEpisodes)
		for _, episode := range out.MissingEpisodes {
			out.MissingEpisodeKeys[episodeKey(1, episode)] = struct{}{}
		}
	} else if out.LocalMediaCount > 0 {
		out.DownloadedEpisodes = 1
		if out.TotalEpisodes == 0 {
			out.TotalEpisodes = 1
		}
	}
	return out
}

func finalizePTSeasonAvailability(sub *model.Subscription, out LocalAvailability) LocalAvailability {
	prefix := fmt.Sprintf("%02dE", sub.SeasonNumber)
	originalCount := len(out.ExistingEpisodeKeys)
	keys := make(map[string]struct{}, originalCount)
	for key := range out.ExistingEpisodeKeys {
		if strings.HasPrefix(key, prefix) {
			keys[key] = struct{}{}
		}
	}
	out.ExistingEpisodeKeys = keys
	_, hasTargetPack := out.SeriesPackSeasons[sub.SeasonNumber]
	_, hasUnqualifiedPack := out.SeriesPackSeasons[0]
	// Legacy/unqualified packs only retain the historical season-one meaning.
	// They cannot prove that a later season is complete.
	if sub.SeasonNumber == 1 && (hasUnqualifiedPack || len(out.SeriesPackSeasons) == 0 && out.HasSeriesPack) {
		hasTargetPack = true
	}
	out.HasSeriesPack = hasTargetPack
	out.SeriesPackSeasons = map[int]struct{}{}
	if hasTargetPack {
		out.SeriesPackSeasons[sub.SeasonNumber] = struct{}{}
	}
	out.LocalMediaCount = len(keys)
	if hasTargetPack {
		out.LocalMediaCount++
	}
	out.InLibrary = out.InLibrary && out.LocalMediaCount > 0
	out.DownloadedEpisodes = len(keys)
	out.TotalEpisodes = sub.TotalEpisodes
	out.MissingEpisodes = missingEpisodesForSeason(keys, out.TotalEpisodes, sub.SeasonNumber)
	out.MissingEpisodeKeys = make(map[string]struct{}, len(out.MissingEpisodes))
	for _, episode := range out.MissingEpisodes {
		out.MissingEpisodeKeys[episodeKey(sub.SeasonNumber, episode)] = struct{}{}
	}
	return out
}

func (s *SubscriptionService) subscriptionBaseSavePath(ctx context.Context, sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	base := strings.TrimSpace(sub.SavePath)
	if base == "" && s != nil {
		base = downloadDefaultSaveRoot(ctx, s.repo)
	}
	return base
}

func subscriptionName(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	return sub.Name
}

func subscriptionFilter(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	return sub.Filter
}

func subscriptionAvailabilityQueries(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	values := []string{availabilityQuery(subscriptionName(sub), subscriptionFilter(sub))}
	for _, keyword := range siteSearchKeywords(sub) {
		values = append(values, cleanAvailabilityTitle(keyword))
	}
	if original := cleanAvailabilityTitle(sub.OriginalName); original != "" {
		values = append(values, original)
	}
	return compactUniqueStrings(values...)
}

func subscriptionMediaType(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	return sub.MediaType
}

func (s *SubscriptionService) downloadPathHasCandidate(ctx context.Context, sub *model.Subscription, title, savePath string) bool {
	savePath = strings.TrimSpace(savePath)
	if savePath == "" {
		savePath = s.subscriptionBaseSavePath(ctx, sub)
	}
	if sub != nil && sub.TMDbID > 0 && !subscriptionCandidatePathMatchesIdentity(sub, title) {
		return false
	}
	query := availabilityQuery(title, subscriptionFilter(sub))
	if savePath == "" || query == "" {
		return false
	}
	wanted := episodeRefsFromTitle(title)
	if len(wanted) == 0 {
		wantSeason, wantEpisode := ParseEpisode(title)
		if wantEpisode > 0 {
			wanted = []episodeRef{{Season: wantSeason, Episode: wantEpisode}}
		}
	}
	found := false
	foundEpisodes := map[string]struct{}{}
	var match func(string) bool
	if sub != nil && sub.TMDbID > 0 {
		match = func(path string) bool { return subscriptionCandidatePathMatchesIdentity(sub, path) }
	}
	_ = scanDownloadPathAnyMatching(ctx, savePath, []string{query}, match, func(path string, season, episode int) bool {
		if len(wanted) == 0 {
			found = true
			return false
		}
		if episode <= 0 {
			return true
		}
		if season <= 0 {
			season = 1
		}
		if refs := episodeRefsFromTitle(path); len(refs) > 0 {
			for _, ref := range refs {
				foundEpisodes[episodeKey(ref.Season, ref.Episode)] = struct{}{}
			}
		} else {
			foundEpisodes[episodeKey(season, episode)] = struct{}{}
		}
		for _, ref := range wanted {
			if _, ok := foundEpisodes[episodeKey(ref.Season, ref.Episode)]; !ok {
				return true
			}
		}
		found = true
		return false
	})
	return found
}
