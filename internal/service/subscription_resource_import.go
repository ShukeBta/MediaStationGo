package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ShukeBta/MediaStationGo/internal/model"
	"go.uber.org/zap"
)

func (s *SubscriptionService) runResourceImportSubscription(ctx context.Context, sub *model.Subscription) (int, error) {
	if err := s.ValidateForSave(ctx, sub); err != nil {
		return 0, err
	}
	library, err := s.repo.Library.FindByID(ctx, sub.LibraryID)
	if err != nil || library == nil {
		return 0, firstNonNilError(err, errors.New("目标媒体库不存在"))
	}
	root, err := s.repo.Library.FindRootByID(ctx, library.ID, sub.LibraryRootID)
	if err != nil || root == nil {
		return 0, firstNonNilError(err, errors.New("目标入库目录不存在"))
	}

	local, err := SubscriptionTargetLocalAvailability(ctx, s.repo, sub)
	if err != nil {
		return 0, err
	}
	pending, err := s.pendingResourceImportAvailability(ctx, sub)
	if err != nil {
		return 0, err
	}
	availability := mergeLocalAvailabilityForSeason(subscriptionSeasonNumber(sub), local, pending)
	if resourceSubscriptionComplete(sub, local) {
		s.finishResourceImportSubscriptionRun(ctx, sub, local, 0)
		return 0, nil
	}

	queries := resourceImportSubscriptionQueries(sub)
	if len(queries) == 0 {
		return 0, errors.New("追更订阅缺少可搜索的作品名称")
	}
	var lastSearchErr error
	searchSucceeded := false
	for _, query := range queries {
		response, searchErr := s.resourceImport.Search(ctx, sub.UserID, *library, *root, ResourceSearchInput{
			Query: query, Source: sub.ResourceSource, Page: 1, PageSize: resourceSearchLimit, RootID: root.ID,
		})
		if searchErr != nil {
			lastSearchErr = searchErr
			if s.log != nil {
				s.log.Warn("resource import subscription search failed",
					zap.String("subscription_id", sub.ID), zap.String("query", query), zap.Error(searchErr))
			}
			continue
		}
		searchSucceeded = true
		candidates := selectResourceImportSubscriptionCandidates(response.Results, sub, availability)
		if len(candidates) == 0 {
			continue
		}
		targetOpenListPath, targetErr := SubscriptionTargetOpenListPath(ctx, s.repo, sub)
		if targetErr != nil {
			return 0, targetErr
		}
		queued, createErr := s.createResourceImportSubscriptionJobs(ctx, sub, *library, *root, response.SessionID, candidates, local, pending, targetOpenListPath)
		s.finishResourceImportSubscriptionRun(ctx, sub, local, queued)
		return queued, createErr
	}
	if !searchSucceeded && lastSearchErr != nil {
		return 0, lastSearchErr
	}
	s.finishResourceImportSubscriptionRun(ctx, sub, local, 0)
	return 0, nil
}

func (s *SubscriptionService) createResourceImportSubscriptionJobs(
	ctx context.Context,
	sub *model.Subscription,
	library model.Library,
	root model.LibraryRoot,
	sessionID string,
	candidates []siteSearchCandidate,
	local LocalAvailability,
	pending LocalAvailability,
	targetOpenListPath string,
) (int, error) {
	if len(candidates) > maxSubscriptionImportsPerRun {
		candidates = candidates[:maxSubscriptionImportsPerRun]
	}
	// One work/season has exactly one active staged-ingest task. A blocked source
	// represents a completed historical conclusion, so it can advance to the next
	// candidate without creating another active task.
	for _, candidate := range candidates {
		index, err := strconv.Atoi(candidate.Item.SiteID)
		if err != nil || index < 0 {
			return 0, errors.New("追更候选索引无效")
		}
		if _, err := s.resourceImport.Create(ctx, sub.UserID, library, root, ResourceImportCreateInput{
			SearchSessionID: sessionID, CandidateIndex: index, RootID: root.ID, SubscriptionID: sub.ID,
			SubscriptionFollow: true, WorkKey: resourceImportSubscriptionWorkKey(sub), Season: subscriptionSeasonNumber(sub),
			ExistingEpisodes:   availabilityEpisodeNumbers(local, subscriptionSeasonNumber(sub)),
			ReservedEpisodes:   availabilityEpisodeNumbers(pending, subscriptionSeasonNumber(sub)),
			TargetOpenListPath: targetOpenListPath, TitleClass: resourceImportCandidateTitleClass(candidate),
		}); err != nil {
			if resourceImportSubscriptionSourceBlocked(err) {
				if s.log != nil {
					s.log.Info("resource import subscription source blocked; trying next candidate", zap.String("subscription_id", sub.ID), zap.String("candidate", candidate.Item.Title))
				}
				continue
			}
			return 0, err
		}
		return 1, nil
	}
	return 0, nil
}

func resourceImportSubscriptionSourceBlocked(err error) bool {
	var pipelineErr *resourcePipelineError
	return errors.As(err, &pipelineErr) && pipelineErr.StatusCode == 409 && pipelineErr.Code == "subscription_source_blocked"
}

func resourceImportSubscriptionWorkKey(sub *model.Subscription) string {
	if sub == nil {
		return ""
	}
	values := []string{sub.OriginalName, sub.Filter, sub.Name}
	for _, value := range values {
		if normalized := normalizeAvailabilityComparable(value); normalized != "" {
			return normalized
		}
	}
	return strings.TrimSpace(sub.ID)
}

func availabilityEpisodeNumbers(value LocalAvailability, season int) []int {
	if season <= 0 {
		season = 1
	}
	values := make([]int, 0, len(value.ExistingEpisodeKeys))
	for key := range value.ExistingEpisodeKeys {
		var itemSeason, episode int
		if _, err := fmt.Sscanf(key, "%02dE%03d", &itemSeason, &episode); err == nil && itemSeason == season && episode > 0 {
			values = append(values, episode)
		}
	}
	return uniqueSortedPositiveInts(values)
}

var resourceCumulativePackRE = regexp.MustCompile(`(?i)(?:更新至|更至|截至|up\s*to|through)\s*(\d{1,4})\s*(?:集|话|話|期|episodes?)?`)

func cumulativePackEndEpisode(text string) int {
	match := resourceCumulativePackRE.FindStringSubmatch(text)
	if len(match) < 2 {
		return 0
	}
	value, _ := strconv.Atoi(match[1])
	if value <= 0 || value > 2000 {
		return 0
	}
	return value
}

func resourceImportCandidateTitleClass(candidate siteSearchCandidate) string {
	text := subscriptionSearchResultText(candidate.Item)
	if resourceCumulativePackRE.MatchString(text) {
		return "cumulative_pack"
	}
	if len(candidateEpisodeNumbers(candidate)) > 1 {
		return "range"
	}
	if candidate.Episode > 0 {
		return "single"
	}
	if candidate.Pack {
		return "season_pack"
	}
	return "unknown"
}

func selectResourceImportSubscriptionCandidates(items []ResourceSearchCandidate, sub *model.Subscription, local LocalAvailability) []siteSearchCandidate {
	results := make([]SearchResult, 0, len(items))
	for _, candidate := range items {
		item := SearchResult{
			SiteName:    "PanSou",
			SiteID:      strconv.Itoa(candidate.Index),
			Title:       candidate.Title,
			Subtitle:    candidate.Subtitle,
			Labels:      strings.TrimSpace(strings.Join([]string{candidate.Summary, candidate.ResourceType, candidate.Resolution}, " ")),
			DownloadURL: fmt.Sprintf("resource-import://candidate/%d", candidate.Index),
			Size:        candidate.SizeBytes,
			Seeders:     candidate.Seeders,
		}
		matchText := subscriptionSearchResultText(item)
		if !subscriptionTitleMatchesQuery(sub, matchText) || !subscriptionSearchResultYearCompatible(sub, matchText) {
			continue
		}
		results = append(results, item)
	}
	stats := siteSearchSelectionStats{Total: len(results)}
	candidates := collectSiteSearchCandidates(results, sub, map[string]struct{}{}, false, &stats)
	for i := range candidates {
		if end := cumulativePackEndEpisode(subscriptionSearchResultText(candidates[i].Item)); end > 0 {
			candidates[i].Season = subscriptionSeasonNumber(sub)
			candidates[i].Episode = 1
			candidates[i].Episodes = make([]int, 0, end)
			for episode := 1; episode <= end; episode++ {
				candidates[i].Episodes = append(candidates[i].Episodes, episode)
			}
			candidates[i].Pack = true
		}
	}
	season := subscriptionSeasonNumber(sub)
	filtered := candidates[:0]
	for _, candidate := range candidates {
		candidateSeason := candidate.Season
		if candidateSeason <= 0 {
			candidateSeason = 1
		}
		if candidate.Episode > 0 && candidateSeason != season {
			continue
		}
		if local.LocalMediaCount > 0 && resourceImportCandidateTitleClass(candidate) == "season_pack" {
			continue
		}
		filtered = append(filtered, candidate)
	}
	selected := make([]siteSearchCandidate, 0, len(filtered))
	for priority := 0; priority <= 2; priority++ {
		group := make([]siteSearchCandidate, 0, len(filtered))
		for _, candidate := range filtered {
			if resourceImportCandidatePriority(candidate) == priority {
				group = append(group, candidate)
			}
		}
		// Keep the generic quality ranking within a candidate type, but do not let
		// a cumulative package consume the missing-episode selection before a
		// single episode or a narrower range has been considered.
		selected = append(selected, selectPreparedSubscriptionCandidates(group, sub, local)...)
	}
	return selected
}

func resourceImportCandidatePriority(candidate siteSearchCandidate) int {
	switch resourceImportCandidateTitleClass(candidate) {
	case "single":
		return 0
	case "range":
		return 1
	default:
		// Cumulative packs, season packs, and unclassified releases are fallbacks.
		return 2
	}
}

func resourceCandidateIsExplicitlyMissing(candidate siteSearchCandidate, availability LocalAvailability) bool {
	episodes := candidateEpisodeNumbers(candidate)
	if len(episodes) == 0 || availability.LocalMediaCount == 0 {
		return false
	}
	season := candidate.Season
	if season <= 0 {
		season = 1
	}
	for _, episode := range episodes {
		if _, exists := availability.ExistingEpisodeKeys[episodeKey(season, episode)]; !exists {
			return true
		}
	}
	return false
}

func resourceImportSubscriptionQueries(sub *model.Subscription) []string {
	if sub == nil {
		return nil
	}
	values := []string{sub.Filter, sub.Name, sub.OriginalName}
	values = append(values, subscriptionFeedAliases(sub)...)
	values = append(values, subscriptionMetadataAliases(sub)...)
	return compactUniqueStrings(values...)
}

func (s *SubscriptionService) pendingResourceImportAvailability(ctx context.Context, sub *model.Subscription) (LocalAvailability, error) {
	out := newSubscriptionSeasonAvailability(sub)
	if s == nil || s.repo == nil || s.repo.DB == nil || sub == nil {
		return out, nil
	}
	var jobs []model.ResourceImportJob
	if err := s.repo.DB.WithContext(ctx).
		Where("library_id = ? AND library_root_id = ? AND status IN ?", sub.LibraryID, sub.LibraryRootID,
			[]string{ResourceImportStatusQueued, ResourceImportStatusRunning}).
		Order("created_at ASC").Find(&jobs).Error; err != nil {
		return out, err
	}
	queries := subscriptionTitleMatchQueries(sub)
	for _, job := range jobs {
		text := resourceImportJobAvailabilityText(job)
		if job.SubscriptionID != sub.ID && !availabilityTitleMatchesAny(text, queries) {
			continue
		}
		addTrustedAvailabilityTitle(text, 0, 0, false, &out)
	}
	return finalizeSubscriptionSeasonAvailability(sub, out), nil
}

func resourceImportJobAvailabilityText(job model.ResourceImportJob) string {
	var candidate ResourceSearchCandidate
	if strings.TrimSpace(job.CandidateJSON) != "" && json.Unmarshal([]byte(job.CandidateJSON), &candidate) == nil {
		return strings.TrimSpace(strings.Join([]string{candidate.Title, candidate.Subtitle, candidate.Summary}, " "))
	}
	return strings.TrimSpace(job.CandidateTitle)
}

func (s *SubscriptionService) finishResourceImportSubscriptionRun(ctx context.Context, sub *model.Subscription, local LocalAvailability, queued int) {
	now := time.Now()
	if err := s.repo.DB.WithContext(ctx).Model(sub).Update("last_run_at", &now).Error; err != nil && s.log != nil {
		s.log.Warn("resource import subscription last_run_at update failed", zap.String("subscription_id", sub.ID), zap.Error(err))
	}
	if err := s.archiveCompletedSubscription(ctx, sub, local); err != nil && s.log != nil {
		s.log.Warn("resource import subscription archive check failed", zap.String("subscription_id", sub.ID), zap.Error(err))
	}
	if queued > 0 && s.hub != nil {
		s.hub.Publish("subscription", map[string]any{"id": sub.ID, "name": sub.Name, "queued": queued})
	}
}

func resourceSubscriptionComplete(sub *model.Subscription, local LocalAvailability) bool {
	if sub == nil || sub.TotalEpisodes <= 0 {
		return false
	}
	return local.DownloadedEpisodes >= sub.TotalEpisodes && len(local.MissingEpisodes) == 0
}

func firstNonNilError(primary, fallback error) error {
	if primary != nil {
		return primary
	}
	return fallback
}
