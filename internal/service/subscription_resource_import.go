package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	active, err := s.hasActiveResourceImport(ctx, sub)
	if err != nil {
		return 0, err
	}
	if active {
		return 0, nil
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

	frontier := firstUnavailableEpisode(availability.ExistingEpisodeKeys, subscriptionSeasonNumber(sub))
	queries := resourceImportSubscriptionQueries(sub, frontier)
	if len(queries) == 0 {
		return 0, errors.New("追更订阅缺少可搜索的作品名称")
	}
	var lastSearchErr error
	searchSucceeded := false
	for _, query := range queries {
		response, searchErr := s.resourceImport.Search(ctx, sub.UserID, *library, *root, ResourceSearchInput{
			Query: query, Source: sub.ResourceSource, Page: 1, PageSize: resourceSearchLimit, RootID: root.ID,
			SubscriptionFollow: true,
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
		queued, createErr := s.createResourceImportSubscriptionJobs(ctx, sub, *library, *root, response.SessionID, candidates, frontier, local, pending, targetOpenListPath)
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
	targetEpisode int,
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
			ExpectedEpisodes:   []int{targetEpisode},
			TargetOpenListPath: targetOpenListPath, TitleClass: "single",
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

func selectResourceImportSubscriptionCandidates(items []ResourceSearchCandidate, sub *model.Subscription, local LocalAvailability) []siteSearchCandidate {
	results := make([]SearchResult, 0, len(items))
	targetEpisode := firstUnavailableEpisode(local.ExistingEpisodeKeys, subscriptionSeasonNumber(sub))
	for _, candidate := range items {
		if !subscriptionTitleMatchesQuery(sub, candidate.Title) || !subscriptionTitleContainsEpisode(candidate.Title, targetEpisode) {
			continue
		}
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
		if !matchesSubscriptionRules(sub, matchText) || !matchesSubscriptionTorrentRules(sub, item) {
			continue
		}
		results = append(results, item)
	}
	candidates := make([]siteSearchCandidate, 0, len(results))
	for _, item := range results {
		candidates = append(candidates, siteSearchCandidate{Item: item})
	}
	return candidates
}

// subscriptionTitleContainsEpisode only checks that the search result title
// contains the requested episode number as a whole numeric token. The search
// query already constrains the work and episode; this deliberately avoids
// inferring a complete episode layout from release-title punctuation.
func subscriptionTitleContainsEpisode(title string, targetEpisode int) bool {
	if targetEpisode <= 0 {
		return false
	}
	for index := 0; index < len(title); {
		if title[index] < '0' || title[index] > '9' {
			index++
			continue
		}
		end := index
		for end < len(title) && title[end] >= '0' && title[end] <= '9' {
			end++
		}
		value, err := strconv.Atoi(title[index:end])
		if err == nil && value == targetEpisode {
			return true
		}
		index = end
	}
	return false
}

func firstUnavailableEpisode(existing map[string]struct{}, season int) int {
	for episode := 1; ; episode++ {
		if _, ok := existing[episodeKey(season, episode)]; !ok {
			return episode
		}
	}
}

func resourceImportSubscriptionQueries(sub *model.Subscription, frontier int) []string {
	if sub == nil {
		return nil
	}
	values := []string{sub.Filter, sub.Name, sub.OriginalName}
	for _, alias := range append(subscriptionFeedAliases(sub), subscriptionMetadataAliases(sub)...) {
		if len(titleYears(alias)) == 0 {
			values = append(values, alias)
		}
	}
	frontierTitle := strings.TrimSpace(sub.Filter)
	if frontierTitle == "" || len(titleYears(frontierTitle)) > 0 {
		frontierTitle = strings.TrimSpace(sub.Name)
	}
	if frontier > 0 && frontierTitle != "" {
		return []string{fmt.Sprintf("%s %d", frontierTitle, frontier)}
	}
	return compactUniqueStrings(values...)
}

func (s *SubscriptionService) pendingResourceImportAvailability(ctx context.Context, sub *model.Subscription) (LocalAvailability, error) {
	out := newSubscriptionSeasonAvailability(sub)
	if s == nil || s.repo == nil || s.repo.DB == nil || sub == nil {
		return out, nil
	}
	var jobs []model.ResourceImportJob
	if err := s.repo.DB.WithContext(ctx).
		Where("library_id = ? AND library_root_id = ?", sub.LibraryID, sub.LibraryRootID).
		Where("status IN ? OR (subscription_id = ? AND subscription_follow = ? AND status IN ?)",
			[]string{ResourceImportStatusQueued, ResourceImportStatusRunning},
			sub.ID, true, []string{ResourceImportStatusCompleted, ResourceImportStatusCompletedWithWarning}).
		Order("created_at ASC").Find(&jobs).Error; err != nil {
		return out, err
	}
	queries := subscriptionTitleMatchQueries(sub)
	for _, job := range jobs {
		if job.SubscriptionID == sub.ID && job.SubscriptionFollow && (job.Status == ResourceImportStatusCompleted || job.Status == ResourceImportStatusCompletedWithWarning) {
			_, _, verified, _ := resourceImportSubscriptionProjection(job.ResultJSON)
			for _, episode := range verified {
				out.ExistingEpisodeKeys[episodeKey(subscriptionSeasonNumber(sub), episode)] = struct{}{}
			}
			continue
		}
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
	sub.CatchUpActive = subscriptionAvailabilityNeedsCatchUp(local, subscriptionSeasonNumber(sub))
	if err := s.repo.DB.WithContext(ctx).Model(sub).Updates(map[string]any{
		"last_run_at":     &now,
		"catch_up_active": sub.CatchUpActive,
	}).Error; err != nil && s.log != nil {
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
